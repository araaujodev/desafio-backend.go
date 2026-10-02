package postgres

import (
	"context"
	"time"

	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

// CreateWallet opens a wallet. A positive initial balance creates an OPENING
// transaction, its credit ledger entry and two outbox events, all in one commit.
func (s *Store) CreateWallet(ctx context.Context, playerID string, initial domain.Money) (*domain.Wallet, error) {
	now := time.Now().UTC()
	w, err := domain.NewWallet(newID(), playerID, initial, now)
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `INSERT INTO wallets (id, player_id, currency, balance, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$6)`, w.ID(), playerID, initial.Currency(), initial.Cents(), w.Version(), now)
	if err != nil {
		if isUnique(err) {
			return nil, ErrWalletExists
		}
		return nil, err
	}

	if initial.IsPositive() {
		txID := newID()
		zero, _ := domain.Zero(initial.Currency())
		entry, err := domain.NewLedgerEntry(newID(), w.ID(), txID, domain.DirectionCredit, initial, zero, initial, now)
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO wager_transactions
			(id, kind, status, wallet_id, player_id, amount, currency, created_at, updated_at)
			VALUES ($1,'OPENING','PROCESSED',$2,$3,$4,$5,$6,$6)`,
			txID, w.ID(), playerID, initial.Cents(), initial.Currency(), now); err != nil {
			return nil, err
		}
		if err = insertLedger(ctx, tx, entry); err != nil {
			return nil, err
		}
		processed := map[string]any{"transactionId": txID, "walletId": w.ID(), "kind": "OPENING", "money": toDTO(initial)}
		if err = insertOutbox(ctx, tx, w.ID(), "WagerTransactionProcessed", processed, now); err != nil {
			return nil, err
		}
		changed := balanceChangedPayload(w.ID(), txID, domain.DirectionCredit, initial, zero, initial, w.Version())
		if err = insertOutbox(ctx, tx, w.ID(), "WalletBalanceChanged", changed, now); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return w, nil
}
