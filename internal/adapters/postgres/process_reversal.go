package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

// checkIdempotency returns a replay result, nil if the key is new, or a conflict.
func checkIdempotency(ctx context.Context, tx pgx.Tx, req domain.ExternalRequest, hash string) (*Result, error) {
	rows, err := tx.Query(ctx, `SELECT idempotency_key, request_hash, result_snapshot
		FROM wager_transactions
		WHERE provider_id=$1 AND (idempotency_key=$2 OR external_transaction_id=$3)`,
		req.ProviderID, req.IdempotencyKey, req.ExternalTransactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snap []byte
	found := false
	for rows.Next() {
		var k, h string
		if err := rows.Scan(&k, &h, &snap); err != nil {
			return nil, err
		}
		if k != req.IdempotencyKey || h != hash {
			return nil, ErrIdempotencyConflict
		}
		found = true
	}
	if err := rows.Err(); err != nil || !found {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(snap, &r); err != nil {
		return nil, err
	}
	r.IdempotentReplay = true
	return &r, nil
}

// decideReversal returns failure code, resulting status and direction.
// When the reversal applies, it also mutates the (locked) wallet aggregate.
func decideReversal(ctx context.Context, tx pgx.Tx, req domain.ExternalRequest, w *domain.Wallet, now time.Time) (string, domain.Status, domain.Direction, error) {
	if w.PlayerID() != req.PlayerID || w.Currency() != req.Money.Currency() {
		return "WALLET_MISMATCH", domain.StatusRejected, "", nil
	}
	ref, found, err := loadRef(ctx, tx, req)
	if err != nil {
		return "", "", "", err
	}
	if !found || ref.status == "PENDING" || ref.status == "PENDING_REFERENCE" {
		return "", domain.StatusPendingReference, "", nil
	}
	if ref.status != "PROCESSED" {
		return "REFERENCE_NOT_SUCCESSFUL", domain.StatusRejected, "", nil
	}
	code, dir := classify(ref, req)
	if code != "" {
		return code, domain.StatusRejected, "", nil
	}
	done, err := alreadyReversed(ctx, tx, req)
	if err != nil {
		return "", "", "", err
	}
	if done {
		return "ALREADY_REVERSED", domain.StatusRejected, "", nil
	}
	if dir == domain.DirectionCredit {
		err = w.Credit(req.Money, now)
	} else {
		err = w.Debit(req.Money, now)
	}
	if errors.Is(err, domain.ErrInsufficientFunds) {
		return "REVERSAL_INSUFFICIENT_FUNDS", domain.StatusRejected, "", nil
	}
	return "", domain.StatusProcessed, dir, err
}

func insertTx(ctx context.Context, tx pgx.Tx, id string, req domain.ExternalRequest, hash string, st domain.Status, code string, snap []byte, now time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO wager_transactions
		(id, kind, status, wallet_id, player_id, amount, currency, provider_id, external_transaction_id,
		 idempotency_key, request_hash, round_id, game_id, reference_external_transaction_id,
		 failure_code, result_snapshot, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)`,
		id, string(req.Kind), string(st), req.WalletID, req.PlayerID, req.Money.Cents(), req.Money.Currency(),
		req.ProviderID, req.ExternalTransactionID, req.IdempotencyKey, hash, req.RoundID, req.GameID,
		nullable(req.ReferenceExternalTxID), nullable(code), snap, now)
	if isUnique(err) {
		return ErrIdempotencyConflict
	}
	return err
}

func reversalOutbox(ctx context.Context, tx pgx.Tx, txID string, req domain.ExternalRequest, st domain.Status, code string, now time.Time) error {
	evt := map[string]any{"transactionId": txID, "walletId": req.WalletID, "providerId": req.ProviderID,
		"externalTransactionId": req.ExternalTransactionID, "kind": string(req.Kind), "money": toDTO(req.Money)}
	typ := "WagerTransactionProcessed"
	switch st {
	case domain.StatusRejected:
		typ = "WagerTransactionRejected"
		evt["failureCode"] = code
	case domain.StatusPendingReference:
		typ = "WagerTransactionPendingReference"
	}
	return insertOutbox(ctx, tx, req.WalletID, typ, evt, now)
}

func applyBalance(ctx context.Context, tx pgx.Tx, txID string, req domain.ExternalRequest, dir domain.Direction, before domain.Money, w *domain.Wallet, now time.Time) error {
	entry, err := domain.NewLedgerEntry(newID(), req.WalletID, txID, dir, req.Money, before, w.Balance(), now)
	if err != nil {
		return err
	}
	if err = insertLedger(ctx, tx, entry); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE wallets SET balance=$1, version=$2, updated_at=$3 WHERE id=$4`,
		w.Balance().Cents(), w.Version(), now, req.WalletID); err != nil {
		return err
	}
	ev := balanceChangedPayload(req.WalletID, txID, dir, req.Money, before, w.Balance(), w.Version())
	return insertOutbox(ctx, tx, req.WalletID, "WalletBalanceChanged", ev, now)
}

// processReversal handles REFUND/ROLLBACK in one SQL transaction under the wallet lock.
func (s *Store) processReversal(ctx context.Context, req domain.ExternalRequest) (Result, error) {
	hash, err := req.RequestHash()
	if err != nil {
		return Result{}, err
	}
	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)

	var playerID, currency string
	var cents, version int64
	var createdAt, updatedAt time.Time
	err = tx.QueryRow(ctx, `SELECT player_id::text, currency, balance, version, created_at, updated_at
		FROM wallets WHERE id=$1 FOR UPDATE`, req.WalletID).
		Scan(&playerID, &currency, &cents, &version, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrWalletNotFound
	}
	if err != nil {
		return Result{}, err
	}
	replay, err := checkIdempotency(ctx, tx, req, hash)
	if err != nil {
		return Result{}, err
	}
	if replay != nil {
		return *replay, nil
	}
	bal, err := domain.NewMoney(cents, currency)
	if err != nil {
		return Result{}, err
	}
	w, err := domain.RehydrateWallet(req.WalletID, playerID, bal, version, createdAt, updatedAt)
	if err != nil {
		return Result{}, err
	}
	before := w.Balance()
	code, status, dir, err := decideReversal(ctx, tx, req, w, now)
	if err != nil {
		return Result{}, err
	}
	txID := newID()
	result := Result{TransactionID: txID, Status: string(status), FailureCode: code, Balance: toDTO(w.Balance())}
	snap, err := json.Marshal(result)
	if err != nil {
		return Result{}, err
	}
	if err = insertTx(ctx, tx, txID, req, hash, status, code, snap, now); err != nil {
		return Result{}, err
	}
	if err = reversalOutbox(ctx, tx, txID, req, status, code, now); err != nil {
		return Result{}, err
	}
	if status == domain.StatusProcessed {
		if err = applyBalance(ctx, tx, txID, req, dir, before, w, now); err != nil {
			return Result{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return result, nil
}
