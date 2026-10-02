package domain

import (
	"errors"
	"testing"
)

func TestLedgerEntryDebitIsValid(t *testing.T) {
	e, err := NewLedgerEntry("e1", "w1", "t1", DirectionDebit,
		brl(t, "80.00"), brl(t, "100.00"), brl(t, "20.00"), now)
	if err != nil || e.Direction() != DirectionDebit {
		t.Fatalf("expected a valid debit entry, got err=%v", err)
	}
}

func TestLedgerEntryCreditIsValid(t *testing.T) {
	if _, err := NewLedgerEntry("e1", "w1", "t1", DirectionCredit,
		brl(t, "10.00"), brl(t, "0.00"), brl(t, "10.00"), now); err != nil {
		t.Fatalf("expected a valid credit entry, got %v", err)
	}
}

func TestLedgerEntryRejectsWrongBalanceAfter(t *testing.T) {
	_, err := NewLedgerEntry("e1", "w1", "t1", DirectionDebit,
		brl(t, "80.00"), brl(t, "100.00"), brl(t, "30.00"), now)
	if !errors.Is(err, ErrInvalidLedgerEntry) {
		t.Fatalf("expected ErrInvalidLedgerEntry, got %v", err)
	}
}

func TestLedgerEntryRejectsInvalidDirection(t *testing.T) {
	_, err := NewLedgerEntry("e1", "w1", "t1", Direction("X"),
		brl(t, "1.00"), brl(t, "1.00"), brl(t, "2.00"), now)
	if !errors.Is(err, ErrInvalidLedgerEntry) {
		t.Fatalf("expected ErrInvalidLedgerEntry, got %v", err)
	}
}

func TestLedgerEntryRejectsZeroMoney(t *testing.T) {
	_, err := NewLedgerEntry("e1", "w1", "t1", DirectionCredit,
		brl(t, "0.00"), brl(t, "1.00"), brl(t, "1.00"), now)
	if !errors.Is(err, ErrInvalidLedgerEntry) {
		t.Fatalf("expected ErrInvalidLedgerEntry, got %v", err)
	}
}

func TestLedgerEntryRejectsCurrencyMismatch(t *testing.T) {
	usd, _ := ParseMoney("10.00", "USD")
	_, err := NewLedgerEntry("e1", "w1", "t1", DirectionCredit,
		usd, brl(t, "0.00"), brl(t, "10.00"), now)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
	}
}
