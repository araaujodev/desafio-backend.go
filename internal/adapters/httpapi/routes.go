package httpapi

import (
	"errors"
	"net/http"

	"github.com/araaujodev/desafio-backend.go/internal/adapters/auth"
	"github.com/araaujodev/desafio-backend.go/internal/adapters/postgres"
)

func (s *Server) registerReads(mux *http.ServeMux) {
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := s.store.Ping(r.Context()); err != nil {
			apiErr(w, http.StatusServiceUnavailable, "NOT_READY")
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("GET /wallets/{id}", s.verifier.Middleware(http.HandlerFunc(s.getWallet)))
	mux.Handle("POST /wallets/{id}/reconciliation", s.verifier.Middleware(http.HandlerFunc(s.reconcile)))
	mux.Handle("GET /wagering/transactions/{id}", s.verifier.Middleware(http.HandlerFunc(s.getTx)))
	mux.Handle("GET /providers/{providerId}/wagering/transactions/{ext}", s.verifier.Middleware(http.HandlerFunc(s.getTxExt)))
}

func (s *Server) internalOnly(w http.ResponseWriter, r *http.Request) bool {
	p, _ := auth.FromContext(r.Context())
	if !p.IsInternal() {
		apiErr(w, http.StatusForbidden, "FORBIDDEN")
		return false
	}
	return true
}

func notFoundOr503(w http.ResponseWriter, err error) {
	if errors.Is(err, postgres.ErrNotFound) {
		apiErr(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	w.Header().Set("Retry-After", "2")
	apiErr(w, http.StatusServiceUnavailable, "UNAVAILABLE")
}

func (s *Server) getWallet(w http.ResponseWriter, r *http.Request) {
	if !s.internalOnly(w, r) {
		return
	}
	v, err := s.store.GetWallet(r.Context(), r.PathValue("id"))
	if err != nil {
		notFoundOr503(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) reconcile(w http.ResponseWriter, r *http.Request) {
	if !s.internalOnly(w, r) {
		return
	}
	v, err := s.store.Reconcile(r.Context(), r.PathValue("id"))
	if err != nil {
		notFoundOr503(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) getTx(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	if p.ProviderID == "" {
		apiErr(w, http.StatusForbidden, "FORBIDDEN")
		return
	}
	v, err := s.store.GetTransaction(r.Context(), p.ProviderID, r.PathValue("id"))
	if err != nil {
		notFoundOr503(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) getTxExt(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	if p.ProviderID == "" || p.ProviderID != r.PathValue("providerId") {
		apiErr(w, http.StatusForbidden, "FORBIDDEN")
		return
	}
	v, err := s.store.GetTransactionByExternal(r.Context(), p.ProviderID, r.PathValue("ext"))
	if err != nil {
		notFoundOr503(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
