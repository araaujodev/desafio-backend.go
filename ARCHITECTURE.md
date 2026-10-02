# Architecture

## Done
- Money: int64 cents, 2-decimal strings, ISO 4217, overflow checks, no floats.
- Wallet aggregate: encapsulated, balance >= 0, version bumps only on balance change, creation separate from rehydration.
- WagerTransaction: 6 kinds, 5 states, terminal states immutable, per-kind rules.
- LedgerEntry: immutable, validates balanceAfter = balanceBefore +/- money.
- Idempotency: canonical JSON (sorted keys, no idempotency key/transport metadata) + SHA-256.
- Schema (PostgreSQL, goose migrations with down): CHECK balance >= 0, UNIQUE (player, currency),
  UNIQUE (provider, external id) and (provider, idempotency key), OPENING vs external CHECK,
  one OPENING per wallet, one PROCESSED reversal per reference, append-only ledger via triggers
  (UPDATE/DELETE/TRUNCATE), inbox (consumer, message id), outbox with lease columns.

## Decisions
- Money stored as BIGINT cents. Concurrency plan: pessimistic `SELECT ... FOR UPDATE` per wallet.
- Replay returns the persisted `result_snapshot`.

## NOT done (explicit)
- HTTP API, OAuth2/OIDC (Keycloak) authentication and authorization.
- ProcessTransaction use case, repositories, Uber Fx composition.
- SQS consumer, outbox publisher, pending-reference worker, reconciliation endpoint.
- Metrics, health checks, integration/concurrency tests, multi-instance tests.
