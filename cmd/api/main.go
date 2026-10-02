package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	"github.com/araaujodev/desafio-backend.go/internal/adapters/auth"
	"github.com/araaujodev/desafio-backend.go/internal/adapters/httpapi"
	"github.com/araaujodev/desafio-backend.go/internal/adapters/postgres"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func newPool(lc fx.Lifecycle) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(),
		env("DATABASE_URL", "postgres://wager:wager_local_pw@localhost:5432/wager?sslmode=disable"))
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error { return pool.Ping(ctx) },
		OnStop:  func(context.Context) error { pool.Close(); return nil },
	})
	return pool, nil
}

func newVerifier(lc fx.Lifecycle) (*auth.Verifier, error) {
	ctx, cancel := context.WithCancel(context.Background())
	issuer := env("OIDC_ISSUER", "http://localhost:8081/realms/wager")
	v, err := auth.NewVerifier(ctx, issuer, issuer+"/protocol/openid-connect/certs")
	if err != nil {
		cancel()
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error { cancel(); return nil }})
	return v, nil
}

func runHTTP(lc fx.Lifecycle, srv *httpapi.Server) {
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ln, err := net.Listen("tcp", env("HTTP_ADDR", ":8080"))
			if err != nil {
				return err
			}
			go hs.Serve(ln)
			return nil
		},
		OnStop: func(ctx context.Context) error { return hs.Shutdown(ctx) },
	})
}

func main() {
	fx.New(
		fx.Provide(newPool, postgres.NewStore, newVerifier, httpapi.NewServer),
		fx.Invoke(runHTTP, runPublisher),
	).Run()
}
