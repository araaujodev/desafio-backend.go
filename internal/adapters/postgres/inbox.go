package postgres

import (
	"context"
	"time"

	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

var ErrInboxHashMismatch = errors.New("message id reused with different payload")

// Envelope is the SQS message body.
type Envelope struct {
	MessageID string          `json:"messageId"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
}

type inData struct {
	ProviderID            string   `json:"providerId"`
	ExternalTransactionID string   `json:"externalTransactionId"`
	IdempotencyKey        string   `json:"idempotencyKey"`
	PlayerID              string   `json:"playerId"`
	WalletID              string   `json:"walletId"`
	RoundID               string   `json:"roundId"`
	GameID                string   `json:"gameId"`
	Kind                  string   `json:"kind"`
	Reference             string   `json:"referenceExternalTransactionId"`
	Money                 MoneyDTO `json:"money"`
}

func hashRaw(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// HandleMessage dedups by (consumer, messageId), verifies the hash on redelivery,
// runs the shared financial use case, then marks the inbox row completed.
func (s *Store) HandleMessage(ctx context.Context, consumer string, raw []byte) (Result, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil || env.MessageID == "" || env.Type != "WagerTransactionRequested" {
		return Result{}, domain.ErrInvalidTransaction
	}
	var d inData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return Result{}, domain.ErrInvalidTransaction
	}
	m, err := domain.ParseMoney(d.Money.Amount, d.Money.Currency)
	if err != nil {
		return Result{}, err
	}
	h := hashRaw(raw)
	var prev string
	err = s.pool.QueryRow(ctx, `INSERT INTO inbox_messages (consumer_name, message_id, payload_hash, received_at)
		VALUES ($1,$2,$3,$4) ON CONFLICT (consumer_name, message_id)
		DO UPDATE SET consumer_name=EXCLUDED.consumer_name RETURNING payload_hash`,
		consumer, env.MessageID, h, time.Now().UTC()).Scan(&prev)
	if err != nil {
		return Result{}, err
	}
	if prev != h {
		return Result{}, ErrInboxHashMismatch
	}
	res, err := s.Process(ctx, domain.ExternalRequest{
		ProviderID: d.ProviderID, ExternalTransactionID: d.ExternalTransactionID, IdempotencyKey: d.IdempotencyKey,
		PlayerID: d.PlayerID, WalletID: d.WalletID, RoundID: d.RoundID, GameID: d.GameID,
		Kind: domain.Kind(d.Kind), Money: m, ReferenceExternalTxID: d.Reference})
	if err != nil {
		return Result{}, err
	}
	_, err = s.pool.Exec(ctx, `UPDATE inbox_messages SET completed_at=now() WHERE consumer_name=$1 AND message_id=$2`,
		consumer, env.MessageID)
	return res, err
}
