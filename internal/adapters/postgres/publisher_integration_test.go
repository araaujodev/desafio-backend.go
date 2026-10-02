//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/araaujodev/desafio-backend.go/internal/adapters/postgres"
	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

type fakeSink struct {
	mu   sync.Mutex
	seen map[string]int
	fail bool
}

func (f *fakeSink) Publish(_ context.Context, e postgres.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("down")
	}
	f.seen[e.EventID]++
	return nil
}

func TestTwoPublishersDoNotDuplicate(t *testing.T) {
	s, pool := setup(t)
	w := newWallet(t, s, "100.00")
	run(t, s, request(t, w, uuid.NewString(), domain.KindBet, "10.00"))
	sink := &fakeSink{seen: map[string]int{}}
	var wg sync.WaitGroup
	for _, name := range []string{"p1", "p2"} {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			_, _ = s.PublishBatch(context.Background(), n, sink, 100, time.Minute)
		}(name)
	}
	wg.Wait()
	for id, n := range sink.seen {
		if n != 1 {
			t.Fatalf("event %s published %d times", id, n)
		}
	}
	var pending int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE published_at IS NULL`).Scan(&pending)
	if pending != 0 {
		t.Fatalf("all events should be published, %d pending", pending)
	}
}

func TestFailedPublishBacksOffAndRecovers(t *testing.T) {
	s, pool := setup(t)
	w := newWallet(t, s, "10.00")
	sink := &fakeSink{seen: map[string]int{}, fail: true}
	_, _ = s.PublishBatch(context.Background(), "p1", sink, 1000, time.Minute)
	var attempts int
	_ = pool.QueryRow(context.Background(), `SELECT max(attempts) FROM outbox_events WHERE aggregate_id=$1`, w.ID()).Scan(&attempts)
	if attempts < 1 {
		t.Fatalf("failed publish must record an attempt, got %d", attempts)
	}
	_, _ = pool.Exec(context.Background(), `UPDATE outbox_events SET next_attempt_at=now() WHERE aggregate_id=$1`, w.ID())
	sink.fail = false
	if _, err := s.PublishBatch(context.Background(), "p2", sink, 1000, time.Minute); err != nil {
		t.Fatal(err)
	}
	var pending int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_events WHERE aggregate_id=$1 AND published_at IS NULL`, w.ID()).Scan(&pending)
	if pending != 0 {
		t.Fatalf("pending after recovery: %d", pending)
	}
}
