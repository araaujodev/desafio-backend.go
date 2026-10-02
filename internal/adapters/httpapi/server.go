package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/araaujodev/desafio-backend.go/internal/adapters/auth"
	"github.com/araaujodev/desafio-backend.go/internal/adapters/postgres"
	"github.com/araaujodev/desafio-backend.go/internal/domain"
)

type Server struct {
	store    *postgres.Store
	verifier *auth.Verifier
}

func NewServer(s *postgres.Store, v *auth.Verifier) *Server { return &Server{store: s, verifier: v} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.Handle("POST /wallets", s.verifier.Middleware(http.HandlerFunc(s.createWallet)))
	mux.Handle("POST /wagering/transactions", s.verifier.Middleware(http.HandlerFunc(s.submit)))
	s.registerReads(mux)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func apiErr(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}

type moneyIn struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func (s *Server) createWallet(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	if !p.IsInternal() {
		apiErr(w, http.StatusForbidden, "FORBIDDEN")
		return
	}
	var in struct {
		PlayerID       string  `json:"playerId"`
		InitialBalance moneyIn `json:"initialBalance"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.PlayerID == "" {
		apiErr(w, http.StatusBadRequest, "INVALID_INPUT")
		return
	}
	m, err := domain.ParseMoney(in.InitialBalance.Amount, in.InitialBalance.Currency)
	if err != nil || m.IsNegative() {
		apiErr(w, http.StatusBadRequest, "INVALID_INPUT")
		return
	}
	wal, err := s.store.CreateWallet(r.Context(), in.PlayerID, m)
	switch {
	case errors.Is(err, postgres.ErrWalletExists):
		apiErr(w, http.StatusConflict, "WALLET_EXISTS")
	case err != nil:
		apiErr(w, http.StatusServiceUnavailable, "UNAVAILABLE")
	default:
		writeJSON(w, http.StatusCreated, map[string]any{
			"id": wal.ID(), "playerId": wal.PlayerID(),
			"balance": map[string]string{"amount": wal.Balance().Amount(), "currency": wal.Currency()},
			"version": wal.Version(),
		})
	}
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	if p.ProviderID == "" {
		apiErr(w, http.StatusForbidden, "FORBIDDEN")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	var in struct {
		ProviderID            string  `json:"providerId"`
		ExternalTransactionID string  `json:"externalTransactionId"`
		PlayerID              string  `json:"playerId"`
		WalletID              string  `json:"walletId"`
		RoundID               string  `json:"roundId"`
		GameID                string  `json:"gameId"`
		Kind                  string  `json:"kind"`
		Money                 moneyIn `json:"money"`
		Reference             string  `json:"referenceExternalTransactionId"`
	}
	if key == "" || json.NewDecoder(r.Body).Decode(&in) != nil {
		apiErr(w, http.StatusBadRequest, "INVALID_INPUT")
		return
	}
	if in.ProviderID != p.ProviderID { // identity comes from the token, never the body
		apiErr(w, http.StatusForbidden, "PROVIDER_MISMATCH")
		return
	}
	m, err := domain.ParseMoney(in.Money.Amount, in.Money.Currency)
	if err != nil || m.IsNegative() {
		apiErr(w, http.StatusBadRequest, "INVALID_INPUT")
		return
	}
	req := domain.ExternalRequest{
		ProviderID: p.ProviderID, ExternalTransactionID: in.ExternalTransactionID, IdempotencyKey: key,
		PlayerID: in.PlayerID, WalletID: in.WalletID, RoundID: in.RoundID, GameID: in.GameID,
		Kind: domain.Kind(in.Kind), Money: m, ReferenceExternalTxID: in.Reference,
	}
	res, err := s.store.Process(r.Context(), req)
	switch {
	case err == nil && res.Status == "REJECTED":
		writeJSON(w, http.StatusUnprocessableEntity, res)
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.Is(err, postgres.ErrIdempotencyConflict):
		apiErr(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT")
	case errors.Is(err, postgres.ErrWalletNotFound):
		apiErr(w, http.StatusNotFound, "WALLET_NOT_FOUND")
	case errors.Is(err, postgres.ErrNotImplemented):
		apiErr(w, http.StatusNotImplemented, "NOT_IMPLEMENTED")
	case isValidation(err):
		apiErr(w, http.StatusBadRequest, "INVALID_INPUT")
	default:
		w.Header().Set("Retry-After", "2")
		apiErr(w, http.StatusServiceUnavailable, "UNAVAILABLE")
	}
}

func isValidation(err error) bool {
	for _, e := range []error{domain.ErrInvalidTransaction, domain.ErrInvalidKind, domain.ErrNonPositiveAmount,
		domain.ErrZeroAmountRequired, domain.ErrReferenceRequired, domain.ErrOpeningNotAllowed} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
