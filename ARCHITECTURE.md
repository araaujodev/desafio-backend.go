# Architecture

## Implemented
- Money: int64 cents, 2-decimal strings, ISO 4217, overflow checks, no floats. Stored as BIGINT.
- Domain (no infra imports): encapsulated Wallet, WagerTransaction state machine, immutable LedgerEntry, canonical-hash idempotency (SHA-256 of sorted-key JSON of business fields; excludes idempotency key and transport metadata).
- Schema (goose, up/down): CHECK balance >= 0, UNIQUE (player,currency), UNIQUE (provider,external id) and (provider,idempotency key), OPENING-vs-external CHECK, one PROCESSED reversal per reference, append-only ledger (triggers on UPDATE/DELETE/TRUNCATE), inbox, outbox.
- Concurrency: pessimistic SELECT ... FOR UPDATE per wallet; independent wallets run in parallel; no global locks.
- Idempotency: persisted in PostgreSQL; same key+hash replays the stored snapshot (original balance); different payload or same (provider, external id) with another key -> 409.
- Reversals: REFUND credits a PROCESSED BET; ROLLBACK is the opposite movement. One PROCESSED reversal per reference (REFUND then ROLLBACK of the same bet is rejected: ALREADY_REVERSED). Codes: INSUFFICIENT_FUNDS, REVERSAL_INSUFFICIENT_FUNDS, REFERENCE_NOT_SUCCESSFUL, REFERENCE_MISMATCH, ALREADY_REVERSED, INVALID_REFERENCE_KIND, WALLET_MISMATCH.
- Auth: Keycloak client_credentials; JWT verified via JWKS (signature, iss, exp). providerId comes from the token (body mismatch -> 403). Wallet endpoints and reconciliation are internal-only. Reads are scoped by provider (404 otherwise).
- Outbox: events written in the same SQL transaction; publisher uses FOR UPDATE SKIP LOCKED + lease + exponential backoff; republication keeps eventId (also the SQS deduplication id). Runs as an Fx worker with graceful stop.
- SQS inbox handler (HandleMessage): dedup by (consumer, messageId), payload-hash check on redelivery, shared financial use case, so HTTP and SQS cannot double-apply an operation.
- Reconciliation (REPEATABLE READ, read-only), /health/live, /health/ready. Fx composition with lifecycle hooks.
- Tests: real PostgreSQL integration tests (two 80.00 bets, 50 parallel duplicates, replay/conflict, reversals, publishers racing, redelivery, HTTP+SQS same operation); scripts/multi.sh runs 3 independent processes. Auth verified manually against real Keycloak.

## HTTP contract
400 INVALID_INPUT, 401 UNAUTHENTICATED, 403 FORBIDDEN/PROVIDER_MISMATCH, 404 NOT_FOUND, 409 IDEMPOTENCY_CONFLICT, 422 rejected (failureCode in body), 202 PENDING_REFERENCE, 503 UNAVAILABLE (+Retry-After).

## NOT done (explicit)
- SQS consumer worker (long polling, DeleteMessage after commit, DLQ handling, SIGTERM drain): only the message handler exists; queues and redrive are provisioned.
- Inbox row is written in its own transaction before the use case, not in the same SQL transaction as the domain changes (safe because the use case is idempotent by key).
- Worker that resumes PENDING_REFERENCE (backoff/TTL): pending rows are durable but never retried.
- Ledger pagination endpoint, metrics, structured JSON logs, OpenTelemetry.
- Automated auth tests and automated kill/restart recovery tests; integration tests do not cover LocalStack/Keycloak (verified manually).
- Dockerfile for the app (the API runs with `go run`); Keycloak in start-dev; `aud` not validated.
