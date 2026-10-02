package domain

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrInvalidAmount    = errors.New("invalid amount")
	ErrInvalidCurrency  = errors.New("invalid currency")
	ErrCurrencyMismatch = errors.New("currency mismatch")
	ErrMoneyOverflow    = errors.New("money overflow")
)

var (
	amountRe   = regexp.MustCompile(`^-?\d+\.\d{2}$`)
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
)

// Money guarda o valor em centavos (int64) e a moeda. Imutável.
type Money struct {
	cents    int64
	currency string
}

func NewMoney(cents int64, currency string) (Money, error) {
	if !currencyRe.MatchString(currency) {
		return Money{}, ErrInvalidCurrency
	}
	return Money{cents: cents, currency: currency}, nil
}

func Zero(currency string) (Money, error) { return NewMoney(0, currency) }

// ParseMoney aceita "25.00" (exatamente 2 casas). Sem float em nenhum ponto.
func ParseMoney(amount, currency string) (Money, error) {
	if !amountRe.MatchString(amount) {
		return Money{}, fmt.Errorf("%w: %q", ErrInvalidAmount, amount)
	}
	digits := strings.Replace(amount, ".", "", 1)
	cents, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("%w: %q", ErrMoneyOverflow, amount)
	}
	return NewMoney(cents, currency)
}

func (m Money) Cents() int64     { return m.cents }
func (m Money) Currency() string { return m.currency }
func (m Money) IsZero() bool     { return m.cents == 0 }
func (m Money) IsNegative() bool { return m.cents < 0 }
func (m Money) IsPositive() bool { return m.cents > 0 }

func (m Money) Add(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, ErrCurrencyMismatch
	}
	if (o.cents > 0 && m.cents > math.MaxInt64-o.cents) ||
		(o.cents < 0 && m.cents < math.MinInt64-o.cents) {
		return Money{}, ErrMoneyOverflow
	}
	return Money{cents: m.cents + o.cents, currency: m.currency}, nil
}

func (m Money) Neg() (Money, error) {
	if m.cents == math.MinInt64 {
		return Money{}, ErrMoneyOverflow
	}
	return Money{cents: -m.cents, currency: m.currency}, nil
}

func (m Money) Sub(o Money) (Money, error) {
	n, err := o.Neg()
	if err != nil {
		return Money{}, err
	}
	return m.Add(n)
}

func (m Money) Compare(o Money) (int, error) {
	if m.currency != o.currency {
		return 0, ErrCurrencyMismatch
	}
	switch {
	case m.cents < o.cents:
		return -1, nil
	case m.cents > o.cents:
		return 1, nil
	}
	return 0, nil
}

// Amount devolve a string decimal com 2 casas ("25.00"), sem usar float.
func (m Money) Amount() string {
	c := m.cents
	if c < 0 {
		u := uint64(-(c + 1)) + 1 // evita overflow em MinInt64
		return fmt.Sprintf("-%d.%02d", u/100, u%100)
	}
	return fmt.Sprintf("%d.%02d", c/100, c%100)
}