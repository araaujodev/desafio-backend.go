package domain

import "errors"

// Business and state errors. They can be compared with errors.Is.
var (
	ErrInsufficientFunds  = errors.New("insufficient funds")
	ErrInvalidTransition  = errors.New("invalid state transition")
	ErrNonPositiveAmount  = errors.New("amount must be positive")
	ErrNegativeBalance    = errors.New("balance cannot be negative")
	ErrInvalidWallet      = errors.New("invalid wallet")
	ErrInvalidLedgerEntry = errors.New("invalid ledger entry")
)
