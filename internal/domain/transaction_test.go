package domain

import (
	"errors"
	"testing"
)

func validReq(t *testing.T, kind Kind, amount string) ExternalRequest {
	t.Helper()
	r := ExternalRequest{
		ProviderID: "provider-a", ExternalTransactionID: "tx-1",
		IdempotencyKey: "provider-a:tx-1", PlayerID: "p1", WalletID: "w1",
		RoundID: "r1", GameID: "g1", Kind: kind, Money: brl(t, amount),
	}
	if kind == KindRefund || kind == KindRollback {
		r.ReferenceExternalTxID = "tx-0"
	}
	return r
}

func TestBetRequiresPositiveAmount(t *testing.T) {
	if err := ValidateExternal(validReq(t, KindBet, "0.00")); !errors.Is(err, ErrNonPositiveAmount) {
		t.Fatalf("expected ErrNonPositiveAmount, got %v", err)
	}
}

func TestLossRequiresZero(t *testing.T) {
	if err := ValidateExternal(validReq(t, KindLoss, "0.00")); err != nil {
		t.Fatalf("LOSS with 0.00 should pass: %v", err)
	}
	if err := ValidateExternal(validReq(t, KindLoss, "1.00")); !errors.Is(err, ErrZeroAmountRequired) {
		t.Fatalf("expected ErrZeroAmountRequired, got %v", err)
	}
}

func TestReversalsRequireReference(t *testing.T) {
	for _, k := range []Kind{KindRefund, KindRollback} {
		r := validReq(t, k, "10.00")
		r.ReferenceExternalTxID = ""
		if err := ValidateExternal(r); !errors.Is(err, ErrReferenceRequired) {
			t.Fatalf("%s: expected ErrReferenceRequired, got %v", k, err)
		}
	}
}

func TestOpeningRejectedExternally(t *testing.T) {
	if err := ValidateExternal(validReq(t, KindOpening, "10.00")); !errors.Is(err, ErrOpeningNotAllowed) {
		t.Fatalf("expected ErrOpeningNotAllowed, got %v", err)
	}
}

func TestInvalidKind(t *testing.T) {
	if err := ValidateExternal(validReq(t, Kind("X"), "10.00")); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("expected ErrInvalidKind, got %v", err)
	}
}

func TestPendingToProcessed(t *testing.T) {
	tx, err := NewWagerTransaction("t1", validReq(t, KindBet, "10.00"), now)
	if err != nil || tx.Status() != StatusPending {
		t.Fatalf("expected PENDING, err=%v", err)
	}
	if err := tx.MarkProcessed(now); err != nil || tx.Status() != StatusProcessed {
		t.Fatalf("expected PROCESSED, err=%v", err)
	}
}

func TestTerminalStateCannotTransition(t *testing.T) {
	tx, _ := NewWagerTransaction("t1", validReq(t, KindBet, "10.00"), now)
	_ = tx.MarkProcessed(now)
	if err := tx.Reject("X", now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if err := tx.MarkPendingReference(now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestPendingReferenceFlow(t *testing.T) {
	tx, _ := NewWagerTransaction("t1", validReq(t, KindRefund, "10.00"), now)
	if err := tx.MarkPendingReference(now); err != nil {
		t.Fatalf("PENDING -> PENDING_REFERENCE should pass: %v", err)
	}
	if err := tx.Reject("REFERENCE_NOT_FOUND", now); err != nil || tx.FailureCode() != "REFERENCE_NOT_FOUND" {
		t.Fatalf("expected REJECTED with code, err=%v", err)
	}
}

func TestRejectRequiresFailureCode(t *testing.T) {
	tx, _ := NewWagerTransaction("t1", validReq(t, KindBet, "10.00"), now)
	if err := tx.Reject("", now); !errors.Is(err, ErrInvalidTransaction) {
		t.Fatalf("expected ErrInvalidTransaction, got %v", err)
	}
}

func TestRehydrateKeepsTerminalState(t *testing.T) {
	tx := RehydrateWagerTransaction("t1", validReq(t, KindBet, "10.00"), StatusProcessed, "", now, now)
	if err := tx.MarkProcessed(now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("rehydrated terminal tx must not transition, got %v", err)
	}
}
