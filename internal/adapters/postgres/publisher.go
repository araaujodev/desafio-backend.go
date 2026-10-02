package postgres

import (
	"context"
	"time"
)

// Event is the envelope published to the destination. EventID is stable across republications.
type Event struct {
	EventID, AggregateID, EventType string
	Version                         int
	Payload                         []byte
	OccurredAt                      time.Time
}

// Sink is the publication destination (SQS in production, fake in tests).
type Sink interface {
	Publish(ctx context.Context, e Event) error
}

// PublishBatch claims pending events with a lease (FOR UPDATE SKIP LOCKED),
// publishes them AFTER they were committed, then marks them published.
// A crash between publish and mark leads to a republication with the same EventID.
func (s *Store) PublishBatch(ctx context.Context, worker string, sink Sink, limit int, lease time.Duration) (int, error) {
	rows, err := s.pool.Query(ctx, `UPDATE outbox_events SET locked_by=$1, locked_until=now()+$2::interval
		WHERE event_id IN (
			SELECT event_id FROM outbox_events
			WHERE published_at IS NULL AND next_attempt_at <= now()
			AND (locked_until IS NULL OR locked_until < now())
			ORDER BY occurred_at LIMIT $3 FOR UPDATE SKIP LOCKED)
		RETURNING event_id::text, aggregate_id, event_type, version, payload, occurred_at`,
		worker, lease.String(), limit)
	if err != nil {
		return 0, err
	}
	var evs []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.EventID, &e.AggregateID, &e.EventType, &e.Version, &e.Payload, &e.OccurredAt); err != nil {
			rows.Close()
			return 0, err
		}
		evs = append(evs, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return s.finish(ctx, sink, evs)
}

func (s *Store) finish(ctx context.Context, sink Sink, evs []Event) (int, error) {
	done := 0
	for _, e := range evs {
		if err := sink.Publish(ctx, e); err != nil {
			// exponential backoff, lock released
			_, _ = s.pool.Exec(ctx, `UPDATE outbox_events SET attempts=attempts+1,
				next_attempt_at=now()+(LEAST(power(2,attempts),300) * interval '1 second'),
				locked_by=NULL, locked_until=NULL WHERE event_id=$1`, e.EventID)
			continue
		}
		if _, err := s.pool.Exec(ctx, `UPDATE outbox_events SET published_at=now(),
			locked_by=NULL, locked_until=NULL WHERE event_id=$1`, e.EventID); err != nil {
			return done, err
		}
		done++
	}
	return done, nil
}
