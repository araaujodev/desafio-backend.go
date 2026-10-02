package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

type refTx struct {
	id, kind, status, wallet, playerID, currency, round string
	amount                                              int64
}

func loadRef(ctx context.Context, tx pgx.Tx, req domain.ExternalRequest) (refTx, bool, error) {
	var r refTx
	err := tx.QueryRow(ctx, `SELECT id::text, kind, status, wallet_id::text, player_id::text,
		currency, COALESCE(round_id,''), amount
		FROM wager_transactions WHERE provider_id=$1 AND external_transaction_id=$2`,
		req.ProviderID, req.ReferenceExternalTxID).
		Scan(&r.id, &r.kind, &r.status, &r.wallet, &r.playerID, &r.currency, &r.round, &r.amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

// classify returns a failure code ("" = ok) and the direction of the movement.
func classify(ref refTx, req domain.ExternalRequest) (string, domain.Direction) {
	if ref.wallet != req.WalletID || ref.playerID != req.PlayerID ||
		ref.currency != req.Money.Currency() || ref.round != req.RoundID ||
		ref.amount != req.Money.Cents() {
		return "REFERENCE_MISMATCH", ""
	}
	switch {
	case ref.kind == "BET":
		return "", domain.DirectionCredit
	case req.Kind == domain.KindRollback && (ref.kind == "WIN" || ref.kind == "REFUND"):
		return "", domain.DirectionDebit
	}
	return "INVALID_REFERENCE_KIND", ""
}

func alreadyReversed(ctx context.Context, tx pgx.Tx, req domain.ExternalRequest) (bool, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM wager_transactions
		WHERE provider_id=$1 AND reference_external_transaction_id=$2
		AND kind IN ('REFUND','ROLLBACK') AND status='PROCESSED'`,
		req.ProviderID, req.ReferenceExternalTxID).Scan(&n)
	return n > 0, err
}
