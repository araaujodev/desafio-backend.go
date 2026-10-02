package main

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"go.uber.org/fx"

	"github.com/araaujodev/desafio-backend.go/internal/adapters/postgres"
)

func runPublisher(lc fx.Lifecycle, store *postgres.Store) {
	endpoint := env("SQS_ENDPOINT", "http://localhost:4566")
	client := sqs.New(sqs.Options{
		Region: "us-east-1", BaseEndpoint: aws.String(endpoint),
		Credentials: aws.NewCredentialsCache(staticCreds{}),
	})
	sink := postgres.SQSSink{Client: client, QueueURL: endpoint + "/000000000000/wallet-events.fifo"}
	host, _ := os.Hostname()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			wg.Add(1)
			go func() {
				defer wg.Done()
				t := time.NewTicker(time.Second)
				defer t.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-t.C:
						_, _ = store.PublishBatch(ctx, host, sink, 50, 30*time.Second)
					}
				}
			}()
			return nil
		},
		OnStop: func(c context.Context) error {
			cancel()
			done := make(chan struct{})
			go func() { wg.Wait(); close(done) }()
			select {
			case <-done:
			case <-c.Done():
			}
			return nil
		},
	})
}

type staticCreds struct{}

func (staticCreds) Retrieve(context.Context) (aws.Credentials, error) {
	return aws.Credentials{AccessKeyID: "test", SecretAccessKey: "test"}, nil
}
