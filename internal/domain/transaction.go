package domain

import "time"

// Kind is the type of a wager transaction.
type Kind string

const (
	KindOpening  Kind = "OPENING"
	KindBet      Kind = "BET"
	KindWin      Kind = "WIN"
	KindLoss     Kind = "LOSS"
	KindRefund   Kind = "REFUND"
	KindRollback Kind = "ROLLBACK"
)

// Status is the state of a wager transaction.
type Status string

const (
	StatusPending          Status = "PENDING"
	StatusPendingReference Status = "PENDING_REFERENCE"
	StatusProcessed        Status = "PROCESSED"
	StatusRejected         Status = "REJECTED"
	StatusFailed           Status = "FAILED"
)

// IsTerminal reports whether no further transitions are allowed.
func (s Status) IsTerminal() bool {
	return s == StatusProcessed || s == StatusRejected || s == StatusFailed
}

// allowed lists the valid transitions of the state machine.
var allowed = map[Status][]Status{
	StatusPending:          {StatusProcessed, StatusPendingReference, StatusRejected, StatusFailed},
	StatusPendingReference: {StatusProcessed, StatusRejected, StatusFailed},
}

// ExternalRequest holds the data of an operation sent by a provider.
type ExternalRequest struct {
	ProviderID            string
	ExternalTransactionID string
	IdempotencyKey        string
	PlayerID              string
	WalletID              string
	RoundID               string
	GameID                string
	Kind                  Kind
	Money                 Money
	ReferenceExternalTxID string
}

// WagerTransaction is a financial operation with a validated state machine.
type WagerTransaction struct {
	id          string
	req         ExternalRequest
	status      Status
	failureCode string
	createdAt   time.Time
	updatedAt   time.Time
}

// ValidateExternal applies the per-kind rules for operations sent by providers.
func ValidateExternal(r ExternalRequest) error {
	if r.ProviderID == "" || r.ExternalTransactionID == "" || r.IdempotencyKey == "" ||
		r.PlayerID == "" || r.WalletID == "" || r.RoundID == "" || r.GameID == "" ||
		r.Money.Currency() == "" {
		return ErrInvalidTransaction
	}
	switch r.Kind {
	case KindOpening:
		return ErrOpeningNotAllowed
	case KindBet, KindWin:
		if !r.Money.IsPositive() {
			return ErrNonPositiveAmount
		}
	case KindLoss:
		if !r.Money.IsZero() {
			return ErrZeroAmountRequired
		}
	case KindRefund, KindRollback:
		if !r.Money.IsPositive() {
			return ErrNonPositiveAmount
		}
		if r.ReferenceExternalTxID == "" {
			return ErrReferenceRequired
		}
	default:
		return ErrInvalidKind
	}
	return nil
}

// NewWagerTransaction creates an external transaction in PENDING.
func NewWagerTransaction(id string, r ExternalRequest, now time.Time) (*WagerTransaction, error) {
	if id == "" {
		return nil, ErrInvalidTransaction
	}
	if err := ValidateExternal(r); err != nil {
		return nil, err
	}
	return &WagerTransaction{id: id, req: r, status: StatusPending, createdAt: now, updatedAt: now}, nil
}

// RehydrateWagerTransaction rebuilds a transaction from persisted data
// without applying any transition.
func RehydrateWagerTransaction(id string, r ExternalRequest, status Status, failureCode string, createdAt, updatedAt time.Time) *WagerTransaction {
	return &WagerTransaction{id: id, req: r, status: status, failureCode: failureCode, createdAt: createdAt, updatedAt: updatedAt}
}

func (t *WagerTransaction) ID() string               { return t.id }
func (t *WagerTransaction) Request() ExternalRequest { return t.req }
func (t *WagerTransaction) Status() Status           { return t.status }
func (t *WagerTransaction) FailureCode() string      { return t.failureCode }

func (t *WagerTransaction) transition(to Status, now time.Time) error {
	if t.status.IsTerminal() {
		return ErrInvalidTransition
	}
	for _, s := range allowed[t.status] {
		if s == to {
			t.status = to
			t.updatedAt = now
			return nil
		}
	}
	return ErrInvalidTransition
}

// MarkProcessed moves the transaction to PROCESSED.
func (t *WagerTransaction) MarkProcessed(now time.Time) error {
	return t.transition(StatusProcessed, now)
}

// MarkPendingReference moves the transaction to PENDING_REFERENCE.
func (t *WagerTransaction) MarkPendingReference(now time.Time) error {
	return t.transition(StatusPendingReference, now)
}

// Reject moves the transaction to REJECTED with a stable failure code.
func (t *WagerTransaction) Reject(code string, now time.Time) error {
	if code == "" {
		return ErrInvalidTransaction
	}
	if err := t.transition(StatusRejected, now); err != nil {
		return err
	}
	t.failureCode = code
	return nil
}

// Fail moves the transaction to FAILED (permanent infrastructure failure).
func (t *WagerTransaction) Fail(code string, now time.Time) error {
	if code == "" {
		return ErrInvalidTransaction
	}
	if err := t.transition(StatusFailed, now); err != nil {
		return err
	}
	t.failureCode = code
	return nil
}
