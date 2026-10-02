package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

// Process applies one external operation in a single SQL transaction:
// wallet row lock -> idempotency check -> domain rule -> transaction row,
// ledger, balance and outbox events -> commit.
func (s *Store) Process(ctx context.Context, req domain.ExternalRequest) (Result, error) {
	if err := domain.ValidateExternal(req); err != nil {
		return Result{}, err
	}
	if req.Kind == domain.KindRefund || req.Kind == domain.KindRollback {
		return Result{}, ErrNotImplemented
	}
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

	// Pessimistic lock per wallet: serializes operations on the same wallet only.
	var playerID, currency string
	var cents, version int64
	var createdAt, updatedAt time.Time
	err = tx.QueryRow(ctx, `SELECT player_id::text, currency, balance, version, created_at, updated_at
		FROM wallets WHERE id = $1 FOR UPDATE`, req.WalletID).
		Scan(&playerID, &currency, &cents, &version, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrWalletNotFound
	}
	if err != nil {
		return Result{}, err
	}

	// Idempotency: same key + same hash replays the stored result; anything else conflicts.
	rows, err := tx.Query(ctx, `SELECT idempotency_key, request_hash, result_snapshot
		FROM wager_transactions
		WHERE provider_id = $1 AND (idempotency_key = $2 OR external_transaction_id = $3)`,
		req.ProviderID, req.IdempotencyKey, req.ExternalTransactionID)
	if err != nil {
		return Result{}, err
	}
	var snap []byte
	found, conflict := false, false
	for rows.Next() {
		var k, h string
		var sn []byte
		if err := rows.Scan(&k, &h, &sn); err != nil {
			rows.Close()
			return Result{}, err
		}
		found = true
		if k != req.IdempotencyKey || h != hash {
			conflict = true
		}
		snap = sn
	}
	rows.Close()
	if rows.Err() != nil {
		return Result{}, rows.Err()
	}
	if conflict {
		return Result{}, ErrIdempotencyConflict
	}
	if found {
		var r Result
		if err := json.Unmarshal(snap, &r); err != nil {
			return Result{}, err
		}
		r.IdempotentReplay = true
		return r, nil
	}

	balance, err := domain.NewMoney(cents, currency)
	if err != nil {
		return Result{}, err
	}
	w, err := domain.RehydrateWallet(req.WalletID, playerID, balance, version, createdAt, updatedAt)
	if err != nil {
		return Result{}, err
	}
	before := w.Balance()

	code := ""
	var opErr error
	if playerID != req.PlayerID || currency != req.Money.Currency() {
		code = "WALLET_MISMATCH"
	} else {
		switch req.Kind {
		case domain.KindBet:
			opErr = w.Debit(req.Money, now)
		case domain.KindWin:
			opErr = w.Credit(req.Money, now)
		}
		if errors.Is(opErr, domain.ErrInsufficientFunds) {
			code, opErr = "INSUFFICIENT_FUNDS", nil
		}
	}
	if opErr != nil {
		return Result{}, opErr
	}
	status := domain.StatusProcessed
	if code != "" {
		status = domain.StatusRejected
	}

	txID := newID()
	result := Result{TransactionID: txID, Status: string(status), FailureCode: code, Balance: toDTO(w.Balance())}
	snapshot, err := json.Marshal(result)
	if err != nil {
		return Result{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO wager_transactions
		(id, kind, status, wallet_id, player_id, amount, currency, provider_id, external_transaction_id,
		 idempotency_key, request_hash, round_id, game_id, reference_external_transaction_id,
		 failure_code, result_snapshot, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)`,
		txID, string(req.Kind), string(status), req.WalletID, req.PlayerID, req.Money.Cents(), req.Money.Currency(),
		req.ProviderID, req.ExternalTransactionID, req.IdempotencyKey, hash, req.RoundID, req.GameID,
		nullable(req.ReferenceExternalTxID), nullable(code), snapshot, now)
	if err != nil {
		if isUnique(err) {
			return Result{}, ErrIdempotencyConflict
		}
		return Result{}, err
	}

	evt := map[string]any{"transactionId": txID, "walletId": req.WalletID, "providerId": req.ProviderID,
		"externalTransactionId": req.ExternalTransactionID, "kind": string(req.Kind), "money": toDTO(req.Money)}
	if status == domain.StatusRejected {
		evt["failureCode"] = code
		err = insertOutbox(ctx, tx, req.WalletID, "WagerTransactionRejected", evt, now)
	} else {
		err = insertOutbox(ctx, tx, req.WalletID, "WagerTransactionProcessed", evt, now)
	}
	if err != nil {
		return Result{}, err
	}

	if status == domain.StatusProcessed && req.Kind != domain.KindLoss {
		dir := domain.DirectionDebit
		if req.Kind == domain.KindWin {
			dir = domain.DirectionCredit
		}
		entry, err := domain.NewLedgerEntry(newID(), req.WalletID, txID, dir, req.Money, before, w.Balance(), now)
		if err != nil {
			return Result{}, err
		}
		if err = insertLedger(ctx, tx, entry); err != nil {
			return Result{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE wallets SET balance=$1, version=$2, updated_at=$3 WHERE id=$4`,
			w.Balance().Cents(), w.Version(), now, req.WalletID); err != nil {
			return Result{}, err
		}
		changed := balanceChangedPayload(req.WalletID, txID, dir, req.Money, before, w.Balance(), w.Version())
		if err = insertOutbox(ctx, tx, req.WalletID, "WalletBalanceChanged", changed, now); err != nil {
			return Result{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return result, nil
}
