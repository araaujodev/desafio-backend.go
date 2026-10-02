package domain

import "errors"

// Transaction validation errors.
var (
	ErrInvalidTransaction = errors.New("invalid transaction")
	ErrInvalidKind        = errors.New("invalid transaction kind")
	ErrReferenceRequired  = errors.New("reference external transaction id is required")
	ErrZeroAmountRequired = errors.New("LOSS requires amount 0.00")
	ErrOpeningNotAllowed  = errors.New("OPENING cannot be submitted externally")
)
