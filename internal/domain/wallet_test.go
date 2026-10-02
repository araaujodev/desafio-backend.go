package domain

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func brl(t *testing.T, amount string) Money {
	t.Helper()
	m, err := ParseMoney(amount, "BRL")
	if err != nil {
		t.Fatalf("invalid test amount %q: %v", amount, err)
	}
	return m
}

func TestNewWalletStartsAtVersionOne(t *testing.T) {
	w, err := NewWallet("w1", "p1", brl(t, "100.00"), now)
	if err != nil || w.Version() != 1 || w.Balance().Amount() != "100.00" {
		t.Fatalf("unexpected wallet: %+v err=%v", w, err)
	}
}

func TestNewWalletAllowsZeroBalance(t *testing.T) {
	if _, err := NewWallet("w1", "p1", brl(t, "0.00"), now); err != nil {
		t.Fatalf("zero balance should be allowed: %v", err)
	}
}

func TestNewWalletRejectsNegativeBalance(t *testing.T) {
	if _, err := NewWallet("w1", "p1", brl(t, "-1.00"), now); !errors.Is(err, ErrNegativeBalance) {
		t.Fatalf("expected ErrNegativeBalance, got %v", err)
	}
}

func TestDebitInsufficientFunds(t *testing.T) {
	w, _ := NewWallet("w1", "p1", brl(t, "100.00"), now)
	if err := w.Debit(brl(t, "80.00"), now); err != nil {
		t.Fatalf("first debit should pass: %v", err)
	}
	if err := w.Debit(brl(t, "80.00"), now); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}
	if w.Balance().Amount() != "20.00" {
		t.Fatalf("balance should stay 20.00, got %s", w.Balance().Amount())
	}
}

func TestVersionOnlyBumpsOnBalanceChange(t *testing.T) {
	w, _ := NewWallet("w1", "p1", brl(t, "100.00"), now)
	_ = w.Debit(brl(t, "500.00"), now) // fails, must not change the version
	if w.Version() != 1 {
		t.Fatalf("version should remain 1, got %d", w.Version())
	}
	_ = w.Credit(brl(t, "10.00"), now)
	if w.Version() != 2 {
		t.Fatalf("version should be 2, got %d", w.Version())
	}
}

func TestMovementCurrencyMismatch(t *testing.T) {
	w, _ := NewWallet("w1", "p1", brl(t, "100.00"), now)
	usd, _ := ParseMoney("1.00", "USD")
	if err := w.Credit(usd, now); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("expected ErrCurrencyMismatch, got %v", err)
	}
}

func TestMovementRejectsNonPositive(t *testing.T) {
	w, _ := NewWallet("w1", "p1", brl(t, "100.00"), now)
	if err := w.Debit(brl(t, "0.00"), now); !errors.Is(err, ErrNonPositiveAmount) {
		t.Fatalf("expected ErrNonPositiveAmount, got %v", err)
	}
}

func TestRehydrateDoesNotChangeState(t *testing.T) {
	w, err := RehydrateWallet("w1", "p1", brl(t, "55.00"), 7, now, now)
	if err != nil || w.Version() != 7 || w.Balance().Amount() != "55.00" {
		t.Fatalf("rehydrate changed state: %+v err=%v", w, err)
	}
}
