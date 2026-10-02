package domain

import (
	"errors"
	"math"
	"testing"
)

func TestParseMoney(t *testing.T) {
	m, err := ParseMoney("25.00", "BRL")
	if err != nil || m.Cents() != 2500 {
		t.Fatalf("esperava 2500, veio %v err=%v", m.Cents(), err)
	}
	invalid := []string{"", "25", "25.1", "25.001", "1e3", "NaN", "Infinity", " 25.00", "25,00", "99999999999999999999.00"}
	for _, bad := range invalid {
		if _, err := ParseMoney(bad, "BRL"); err == nil {
			t.Errorf("%q deveria ser rejeitado", bad)
		}
	}
}

func TestInvalidCurrency(t *testing.T) {
	if _, err := ParseMoney("1.00", "brl"); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("esperava ErrInvalidCurrency, veio %v", err)
	}
}

func TestAddCurrencyMismatch(t *testing.T) {
	a, _ := ParseMoney("1.00", "BRL")
	b, _ := ParseMoney("1.00", "USD")
	if _, err := a.Add(b); !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("esperava ErrCurrencyMismatch, veio %v", err)
	}
}

func TestSubAndAmount(t *testing.T) {
	a, _ := ParseMoney("100.00", "BRL")
	b, _ := ParseMoney("80.00", "BRL")
	r, err := a.Sub(b)
	if err != nil || r.Amount() != "20.00" {
		t.Fatalf("esperava 20.00, veio %s err=%v", r.Amount(), err)
	}
}

func TestAddOverflow(t *testing.T) {
	a, _ := NewMoney(math.MaxInt64, "BRL")
	b, _ := NewMoney(1, "BRL")
	if _, err := a.Add(b); !errors.Is(err, ErrMoneyOverflow) {
		t.Fatalf("esperava ErrMoneyOverflow, veio %v", err)
	}
}
