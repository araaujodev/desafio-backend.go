package postgres

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SQSSink publishes outbox events to a FIFO queue. The eventId is both the
// deduplication id and part of the body, so republications keep the same id.
type SQSSink struct {
	Client   *sqs.Client
	QueueURL string
}

func (s SQSSink) Publish(ctx context.Context, e Event) error {
	body, err := json.Marshal(map[string]any{
		"eventId": e.EventID, "eventType": e.EventType, "aggregateId": e.AggregateID,
		"occurredAt": e.OccurredAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"version":    e.Version, "data": json.RawMessage(e.Payload),
	})
	if err != nil {
		return err
	}
	_, err = s.Client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl: aws.String(s.QueueURL), MessageBody: aws.String(string(body)),
		MessageGroupId:         aws.String(e.AggregateID),
		MessageDeduplicationId: aws.String(e.EventID),
	})
	return err
}
