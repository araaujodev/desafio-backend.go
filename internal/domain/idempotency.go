package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CanonicalJSON returns the deterministic JSON used to compute the request hash.
// Only business fields are included (never the idempotency key or transport metadata).
// Keys are sorted alphabetically (encoding/json sorts map keys).
func (r ExternalRequest) CanonicalJSON() ([]byte, error) {
	doc := map[string]any{
		"providerId":            r.ProviderID,
		"externalTransactionId": r.ExternalTransactionID,
		"playerId":              r.PlayerID,
		"walletId":              r.WalletID,
		"roundId":               r.RoundID,
		"gameId":                r.GameID,
		"kind":                  string(r.Kind),
		"money": map[string]any{
			"amount":   r.Money.Amount(),
			"currency": r.Money.Currency(),
		},
	}
	if r.ReferenceExternalTxID != "" {
		doc["referenceExternalTransactionId"] = r.ReferenceExternalTxID
	}
	return json.Marshal(doc)
}

// RequestHash returns the SHA-256 (hex) of the canonical JSON.
func (r ExternalRequest) RequestHash() (string, error) {
	b, err := r.CanonicalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
