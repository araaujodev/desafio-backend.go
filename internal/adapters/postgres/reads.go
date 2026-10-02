package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("not found")

type WalletView struct {
	ID       string   `json:"id"`
	PlayerID string   `json:"playerId"`
	Balance  MoneyDTO `json:"balance"`
	Version  int64    `json:"version"`
}

type TxView struct {
	TransactionID string          `json:"transactionId"`
	Status        string          `json:"status"`
	Kind          string          `json:"kind"`
	FailureCode   string          `json:"failureCode,omitempty"`
	Result        json.RawMessage `json:"result,omitempty"`
}

type Reconciliation struct {
	WalletID          string   `json:"walletId"`
	StoredBalance     MoneyDTO `json:"storedBalance"`
	CalculatedBalance MoneyDTO `json:"calculatedBalance"`
	Difference        MoneyDTO `json:"difference"`
	Consistent        bool     `json:"consistent"`
	CheckedEntries    int64    `json:"checkedEntries"`
}

func cents(c int64, cur string) MoneyDTO {
	neg := c < 0
	if neg {
		c = -c
	}
	s := ""
	if neg {
		s = "-"
	}
	return MoneyDTO{Amount: s + itoa(c/100) + "." + pad2(c%100), Currency: cur}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func (s *Store) GetWallet(ctx context.Context, id string) (WalletView, error) {
	var v WalletView
	var c int64
	var cur string
	err := s.pool.QueryRow(ctx, `SELECT id::text, player_id::text, balance, currency, version
		FROM wallets WHERE id=$1`, id).Scan(&v.ID, &v.PlayerID, &c, &cur, &v.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	v.Balance = cents(c, cur)
	return v, err
}

// GetTransaction is always scoped by provider: another provider's data looks like 404.
func (s *Store) GetTransaction(ctx context.Context, providerID, txID string) (TxView, error) {
	return s.getTx(ctx, `WHERE id::text=$1 AND provider_id=$2`, txID, providerID)
}

func (s *Store) GetTransactionByExternal(ctx context.Context, providerID, ext string) (TxView, error) {
	return s.getTx(ctx, `WHERE external_transaction_id=$1 AND provider_id=$2`, ext, providerID)
}

func (s *Store) getTx(ctx context.Context, where string, a, b string) (TxView, error) {
	var v TxView
	var fc *string
	var snap []byte
	err := s.pool.QueryRow(ctx, `SELECT id::text, status, kind, failure_code, result_snapshot
		FROM wager_transactions `+where, a, b).Scan(&v.TransactionID, &v.Status, &v.Kind, &fc, &snap)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if fc != nil {
		v.FailureCode = *fc
	}
	v.Result = snap
	return v, err
}

// Reconcile rebuilds the balance from the ledger in a consistent read-only snapshot.
func (s *Store) Reconcile(ctx context.Context, walletID string) (Reconciliation, error) {
	var r Reconciliation
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return r, err
	}
	defer tx.Rollback(ctx)
	var stored, calc, n int64
	var cur string
	err = tx.QueryRow(ctx, `SELECT balance, currency FROM wallets WHERE id=$1`, walletID).Scan(&stored, &cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(CASE WHEN direction='CREDIT' THEN amount ELSE -amount END),0), count(*)
		FROM ledger_entries WHERE wallet_id=$1`, walletID).Scan(&calc, &n)
	if err != nil {
		return r, err
	}
	r = Reconciliation{WalletID: walletID, StoredBalance: cents(stored, cur), CalculatedBalance: cents(calc, cur),
		Difference: cents(stored-calc, cur), Consistent: stored == calc, CheckedEntries: n}
	return r, nil
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
