package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

var (
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletExists        = errors.New("wallet already exists for player and currency")
	ErrNotImplemented      = errors.New("operation kind not implemented yet")
)

// Store implements the financial use cases on top of PostgreSQL.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type MoneyDTO struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func toDTO(m domain.Money) MoneyDTO { return MoneyDTO{Amount: m.Amount(), Currency: m.Currency()} }

// Result is the persisted outcome returned to the caller (and replayed).
type Result struct {
	TransactionID    string   `json:"transactionId"`
	Status           string   `json:"status"`
	FailureCode      string   `json:"failureCode,omitempty"`
	Balance          MoneyDTO `json:"balance"`
	IdempotentReplay bool     `json:"idempotentReplay"`
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// insertOutbox writes an event in the same SQL transaction as the state change.
func insertOutbox(ctx context.Context, tx pgx.Tx, aggregateID, eventType string, payload any, now time.Time) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events
		(event_id, aggregate_id, event_type, version, payload, occurred_at)
		VALUES ($1,$2,$3,1,$4,$5)`, newID(), aggregateID, eventType, b, now)
	return err
}

func insertLedger(ctx context.Context, tx pgx.Tx, e domain.LedgerEntry) error {
	_, err := tx.Exec(ctx, `INSERT INTO ledger_entries
		(id, wallet_id, transaction_id, direction, amount, balance_before, balance_after, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.ID(), e.WalletID(), e.TransactionID(), string(e.Direction()),
		e.Money().Cents(), e.BalanceBefore().Cents(), e.BalanceAfter().Cents(), e.CreatedAt())
	return err
}

func balanceChangedPayload(walletID, txID string, d domain.Direction, m, before, after domain.Money, version int64) map[string]any {
	return map[string]any{
		"walletId": walletID, "transactionId": txID, "direction": string(d),
		"money": toDTO(m), "balanceBefore": toDTO(before), "balanceAfter": toDTO(after),
		"walletVersion": version,
	}
}
