package domain

import "time"

// Wallet is the financial aggregate root. State is private so every change
// goes through methods that preserve the invariants.
type Wallet struct {
	id        string
	playerID  string
	balance   Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

// NewWallet creates a brand-new wallet with version 1.
// The initial balance may be zero, but never negative.
func NewWallet(id, playerID string, initial Money, now time.Time) (*Wallet, error) {
	if id == "" || playerID == "" || initial.Currency() == "" {
		return nil, ErrInvalidWallet
	}
	if initial.IsNegative() {
		return nil, ErrNegativeBalance
	}
	return &Wallet{
		id:        id,
		playerID:  playerID,
		balance:   initial,
		version:   1,
		createdAt: now,
		updatedAt: now,
	}, nil
}

// RehydrateWallet rebuilds a wallet from persisted data.
// It does not apply movements, emit events or change the version.
func RehydrateWallet(id, playerID string, balance Money, version int64, createdAt, updatedAt time.Time) (*Wallet, error) {
	if id == "" || playerID == "" || balance.Currency() == "" || version < 1 {
		return nil, ErrInvalidWallet
	}
	if balance.IsNegative() {
		return nil, ErrNegativeBalance
	}
	return &Wallet{
		id: id, playerID: playerID, balance: balance,
		version: version, createdAt: createdAt, updatedAt: updatedAt,
	}, nil
}

func (w *Wallet) ID() string           { return w.id }
func (w *Wallet) PlayerID() string     { return w.playerID }
func (w *Wallet) Balance() Money       { return w.balance }
func (w *Wallet) Currency() string     { return w.balance.Currency() }
func (w *Wallet) Version() int64       { return w.version }
func (w *Wallet) CreatedAt() time.Time { return w.createdAt }
func (w *Wallet) UpdatedAt() time.Time { return w.updatedAt }

// Debit removes money from the wallet. The balance never goes below zero.
func (w *Wallet) Debit(m Money, now time.Time) error {
	if err := w.checkMovement(m); err != nil {
		return err
	}
	cmp, err := w.balance.Compare(m)
	if err != nil {
		return err
	}
	if cmp < 0 {
		return ErrInsufficientFunds
	}
	next, err := w.balance.Sub(m)
	if err != nil {
		return err
	}
	w.apply(next, now)
	return nil
}

// Credit adds money to the wallet.
func (w *Wallet) Credit(m Money, now time.Time) error {
	if err := w.checkMovement(m); err != nil {
		return err
	}
	next, err := w.balance.Add(m)
	if err != nil {
		return err
	}
	w.apply(next, now)
	return nil
}

func (w *Wallet) checkMovement(m Money) error {
	if m.Currency() != w.balance.Currency() {
		return ErrCurrencyMismatch
	}
	if !m.IsPositive() {
		return ErrNonPositiveAmount
	}
	return nil
}

// apply updates the balance and bumps the version (only when money moves).
func (w *Wallet) apply(next Money, now time.Time) {
	w.balance = next
	w.version++
	w.updatedAt = now
}
