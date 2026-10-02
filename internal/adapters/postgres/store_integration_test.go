//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/araaujodev/desafio-backend.go/internal/adapters/postgres"
	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

func setup(t *testing.T) (*postgres.Store, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return postgres.NewStore(pool), pool
}

func money(t *testing.T, a string) domain.Money {
	t.Helper()
	m, err := domain.ParseMoney(a, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func newWallet(t *testing.T, s *postgres.Store, amount string) *domain.Wallet {
	t.Helper()
	w, err := s.CreateWallet(context.Background(), uuid.NewString(), money(t, amount))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func request(t *testing.T, w *domain.Wallet, ext string, kind domain.Kind, amount string) domain.ExternalRequest {
	t.Helper()
	return domain.ExternalRequest{
		ProviderID: "provider-a", ExternalTransactionID: ext, IdempotencyKey: "provider-a:" + ext,
		PlayerID: w.PlayerID(), WalletID: w.ID(), RoundID: "r1", GameID: "g1",
		Kind: kind, Money: money(t, amount),
	}
}

func count(t *testing.T, pool *pgxpool.Pool, q, id string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func balance(t *testing.T, pool *pgxpool.Pool, id string) int64 {
	t.Helper()
	var c int64
	if err := pool.QueryRow(context.Background(), `SELECT balance FROM wallets WHERE id=$1`, id).Scan(&c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTwoBetsOf80OnBalance100(t *testing.T) {
	s, pool := setup(t)
	w := newWallet(t, s, "100.00")
	results := make([]postgres.Result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := s.Process(context.Background(), request(t, w, uuid.NewString(), domain.KindBet, "80.00"))
			if err != nil {
				t.Error(err)
			}
			results[i] = r
		}(i)
	}
	wg.Wait()

	processed, rejected := 0, 0
	for _, r := range results {
		switch {
		case r.Status == "PROCESSED":
			processed++
		case r.Status == "REJECTED" && r.FailureCode == "INSUFFICIENT_FUNDS":
			rejected++
		}
	}
	if processed != 1 || rejected != 1 {
		t.Fatalf("want 1 processed + 1 rejected, got %d/%d", processed, rejected)
	}
	if b := balance(t, pool, w.ID()); b != 2000 {
		t.Fatalf("final balance should be 20.00, got %d cents", b)
	}
	if n := count(t, pool, `SELECT count(*) FROM ledger_entries WHERE wallet_id=$1 AND direction='DEBIT'`, w.ID()); n != 1 {
		t.Fatalf("want a single debit in the ledger, got %d", n)
	}
}

func TestSameBet50TimesInParallel(t *testing.T) {
	s, pool := setup(t)
	w := newWallet(t, s, "100.00")
	req := request(t, w, uuid.NewString(), domain.KindBet, "25.00")
	results := make([]postgres.Result, 50)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := s.Process(context.Background(), req)
			if err != nil {
				t.Error(err)
			}
			results[i] = r
		}(i)
	}
	wg.Wait()

	originals := 0
	for _, r := range results {
		if !r.IdempotentReplay {
			originals++
		}
		if r.TransactionID != results[0].TransactionID || r.Balance.Amount != "75.00" {
			t.Fatalf("replays must return the original result, got %+v", r)
		}
	}
	if originals != 1 {
		t.Fatalf("want exactly 1 non-replay result, got %d", originals)
	}
	if n := count(t, pool, `SELECT count(*) FROM ledger_entries WHERE wallet_id=$1 AND direction='DEBIT'`, w.ID()); n != 1 {
		t.Fatalf("want a single debit, got %d", n)
	}
}

func TestReplayKeepsOriginalBalanceAndConflictOnDifferentPayload(t *testing.T) {
	s, _ := setup(t)
	w := newWallet(t, s, "100.00")
	ext := uuid.NewString()
	first, err := s.Process(context.Background(), request(t, w, ext, domain.KindBet, "10.00"))
	if err != nil || first.Balance.Amount != "90.00" {
		t.Fatalf("first: %+v err=%v", first, err)
	}
	if _, err := s.Process(context.Background(), request(t, w, uuid.NewString(), domain.KindBet, "5.00")); err != nil {
		t.Fatal(err)
	}
	replay, err := s.Process(context.Background(), request(t, w, ext, domain.KindBet, "10.00"))
	if err != nil || !replay.IdempotentReplay || replay.Balance.Amount != "90.00" {
		t.Fatalf("replay must keep the original balance: %+v err=%v", replay, err)
	}
	_, err = s.Process(context.Background(), request(t, w, ext, domain.KindBet, "11.00"))
	if !errors.Is(err, postgres.ErrIdempotencyConflict) {
		t.Fatalf("want ErrIdempotencyConflict, got %v", err)
	}
}

func TestDuplicateWalletForPlayerConflicts(t *testing.T) {
	s, _ := setup(t)
	p := uuid.NewString()
	if _, err := s.CreateWallet(context.Background(), p, money(t, "1.00")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateWallet(context.Background(), p, money(t, "1.00")); !errors.Is(err, postgres.ErrWalletExists) {
		t.Fatalf("want ErrWalletExists, got %v", err)
	}
}
