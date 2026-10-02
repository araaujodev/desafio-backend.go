//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/araaujodev/desafio-backend.go/internal/adapters/postgres"
	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

func rev(t *testing.T, w *domain.Wallet, kind domain.Kind, amount, ref string) domain.ExternalRequest {
	t.Helper()
	r := request(t, w, uuid.NewString(), kind, amount)
	r.ReferenceExternalTxID = ref
	return r
}

func run(t *testing.T, s *postgres.Store, r domain.ExternalRequest) postgres.Result {
	t.Helper()
	res, err := s.Process(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestRefundThenRollbackIsRejected(t *testing.T) {
	s, pool := setup(t)
	w := newWallet(t, s, "100.00")
	bet := request(t, w, uuid.NewString(), domain.KindBet, "30.00")
	run(t, s, bet)
	r := run(t, s, rev(t, w, domain.KindRefund, "30.00", bet.ExternalTransactionID))
	if r.Status != "PROCESSED" || r.Balance.Amount != "100.00" {
		t.Fatalf("refund: %+v", r)
	}
	r = run(t, s, rev(t, w, domain.KindRollback, "30.00", bet.ExternalTransactionID))
	if r.Status != "REJECTED" || r.FailureCode != "ALREADY_REVERSED" {
		t.Fatalf("rollback after refund must be rejected: %+v", r)
	}
	if b := balance(t, pool, w.ID()); b != 10000 {
		t.Fatalf("balance should be 100.00, got %d", b)
	}
}

func TestReversalBeforeReferenceIsPending(t *testing.T) {
	s, pool := setup(t)
	w := newWallet(t, s, "50.00")
	r := run(t, s, rev(t, w, domain.KindRefund, "10.00", "does-not-exist"))
	if r.Status != "PENDING_REFERENCE" || r.Balance.Amount != "50.00" {
		t.Fatalf("want PENDING_REFERENCE, got %+v", r)
	}
	if b := balance(t, pool, w.ID()); b != 5000 {
		t.Fatalf("balance must not change, got %d", b)
	}
}

func TestRollbackOfWinWithoutFundsHasOwnCode(t *testing.T) {
	s, _ := setup(t)
	w := newWallet(t, s, "0.00")
	win := request(t, w, uuid.NewString(), domain.KindWin, "50.00")
	run(t, s, win)
	run(t, s, request(t, w, uuid.NewString(), domain.KindBet, "50.00"))
	r := run(t, s, rev(t, w, domain.KindRollback, "50.00", win.ExternalTransactionID))
	if r.Status != "REJECTED" || r.FailureCode != "REVERSAL_INSUFFICIENT_FUNDS" {
		t.Fatalf("want REVERSAL_INSUFFICIENT_FUNDS, got %+v", r)
	}
}

func TestConcurrentReversalsOfSameBetApplyOnce(t *testing.T) {
	s, pool := setup(t)
	w := newWallet(t, s, "100.00")
	bet := request(t, w, uuid.NewString(), domain.KindBet, "20.00")
	run(t, s, bet)
	results := make([]postgres.Result, 10)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			kind := domain.KindRefund
			if i%2 == 1 {
				kind = domain.KindRollback
			}
			results[i], _ = s.Process(context.Background(), rev(t, w, kind, "20.00", bet.ExternalTransactionID))
		}(i)
	}
	wg.Wait()
	processed := 0
	for _, r := range results {
		if r.Status == "PROCESSED" {
			processed++
		}
	}
	if processed != 1 {
		t.Fatalf("exactly one reversal must apply, got %d", processed)
	}
	if b := balance(t, pool, w.ID()); b != 10000 {
		t.Fatalf("balance should be 100.00, got %d", b)
	}
	if n := count(t, pool, `SELECT count(*) FROM ledger_entries WHERE wallet_id=$1 AND direction='CREDIT'`, w.ID()); n != 2 {
		t.Fatalf("want opening + 1 reversal credit, got %d", n)
	}
}
