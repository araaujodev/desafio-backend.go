//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/araaujodev/desafio-backend.go/internal/adapters/postgres"
)

func msg(id, ext, wallet, player, amount string) []byte {
	return []byte(fmt.Sprintf(`{"messageId":"%s","type":"WagerTransactionRequested","data":{"providerId":"provider-a","externalTransactionId":"%s","idempotencyKey":"provider-a:%s","playerId":"%s","walletId":"%s","roundId":"r1","gameId":"g1","kind":"BET","money":{"amount":"%s","currency":"BRL"}}}`, id, ext, ext, player, wallet, amount))
}

func TestRedeliveredMessageDebitsOnce(t *testing.T) {
	s, pool := setup(t)
	w := newWallet(t, s, "100.00")
	m := msg(uuid.NewString(), uuid.NewString(), w.ID(), w.PlayerID(), "25.00")
	first, err := s.HandleMessage(context.Background(), "wager-consumer", m)
	if err != nil || first.Balance.Amount != "75.00" {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := s.HandleMessage(context.Background(), "wager-consumer", m)
	if err != nil || !second.IdempotentReplay || second.Balance.Amount != "75.00" {
		t.Fatalf("redelivery must replay: %+v %v", second, err)
	}
	if b := balance(t, pool, w.ID()); b != 7500 {
		t.Fatalf("balance should be 75.00, got %d", b)
	}
}

func TestSameMessageIdDifferentPayloadFails(t *testing.T) {
	s, _ := setup(t)
	w := newWallet(t, s, "100.00")
	id := uuid.NewString()
	if _, err := s.HandleMessage(context.Background(), "wager-consumer", msg(id, uuid.NewString(), w.ID(), w.PlayerID(), "10.00")); err != nil {
		t.Fatal(err)
	}
	_, err := s.HandleMessage(context.Background(), "wager-consumer", msg(id, uuid.NewString(), w.ID(), w.PlayerID(), "11.00"))
	if !errors.Is(err, postgres.ErrInboxHashMismatch) {
		t.Fatalf("want ErrInboxHashMismatch, got %v", err)
	}
}

func TestSameOperationViaHTTPAndSQSDebitsOnce(t *testing.T) {
	s, pool := setup(t)
	w := newWallet(t, s, "100.00")
	ext := uuid.NewString()
	run(t, s, request(t, w, ext, "BET", "25.00"))
	r, err := s.HandleMessage(context.Background(), "wager-consumer", msg(uuid.NewString(), ext, w.ID(), w.PlayerID(), "25.00"))
	if err != nil || !r.IdempotentReplay {
		t.Fatalf("SQS after HTTP must replay: %+v %v", r, err)
	}
	if b := balance(t, pool, w.ID()); b != 7500 {
		t.Fatalf("single debit expected, got %d", b)
	}
}
