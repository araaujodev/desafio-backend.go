package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

type ctxKey struct{}

// Principal is the authenticated identity extracted from a verified token.
type Principal struct {
	ProviderID string
	Role       string
}

func (p Principal) IsInternal() bool { return p.Role == "internal" }

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// Verifier validates tokens against the IdP JWKS (keys are cached and refreshed).
type Verifier struct {
	issuer string
	cache  *jwk.Cache
	jwks   string
}

func NewVerifier(ctx context.Context, issuer, jwksURL string) (*Verifier, error) {
	c := jwk.NewCache(ctx)
	if err := c.Register(jwksURL, jwk.WithMinRefreshInterval(5*time.Minute)); err != nil {
		return nil, err
	}
	return &Verifier{issuer: issuer, cache: c, jwks: jwksURL}, nil
}

func (v *Verifier) Verify(ctx context.Context, raw string) (Principal, error) {
	set, err := v.cache.Get(ctx, v.jwks)
	if err != nil {
		return Principal{}, err
	}
	tok, err := jwt.Parse([]byte(raw), jwt.WithKeySet(set), jwt.WithIssuer(v.issuer),
		jwt.WithValidate(true))
	if err != nil {
		return Principal{}, err
	}
	var p Principal
	if x, ok := tok.Get("providerId"); ok {
		p.ProviderID, _ = x.(string)
	}
	if x, ok := tok.Get("role"); ok {
		p.Role, _ = x.(string)
	}
	return p, nil
}

// Middleware rejects missing/invalid/expired tokens with 401 before any handler runs.
func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, `{"code":"UNAUTHENTICATED"}`, http.StatusUnauthorized)
			return
		}
		p, err := v.Verify(r.Context(), strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			http.Error(w, `{"code":"UNAUTHENTICATED"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)))
	})
}
