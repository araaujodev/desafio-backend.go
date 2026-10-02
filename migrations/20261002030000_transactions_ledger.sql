-- +goose Up
CREATE TABLE wager_transactions (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('OPENING','BET','WIN','LOSS','REFUND','ROLLBACK')),
    status TEXT NOT NULL CHECK (status IN ('PENDING','PENDING_REFERENCE','PROCESSED','REJECTED','FAILED')),
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    player_id UUID NOT NULL,
    amount BIGINT NOT NULL CHECK (amount >= 0),
    currency CHAR(3) NOT NULL,
    provider_id TEXT,
    external_transaction_id TEXT,
    idempotency_key TEXT,
    request_hash TEXT,
    round_id TEXT,
    game_id TEXT,
    reference_external_transaction_id TEXT,
    failure_code TEXT,
    result_snapshot JSONB,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT origin_check CHECK (
        (kind = 'OPENING' AND provider_id IS NULL AND external_transaction_id IS NULL
            AND idempotency_key IS NULL AND request_hash IS NULL AND round_id IS NULL
            AND game_id IS NULL AND reference_external_transaction_id IS NULL)
        OR
        (kind <> 'OPENING' AND provider_id IS NOT NULL AND external_transaction_id IS NOT NULL
            AND idempotency_key IS NOT NULL AND request_hash IS NOT NULL
            AND round_id IS NOT NULL AND game_id IS NOT NULL)
    ),
    CONSTRAINT tx_provider_external_uq UNIQUE (provider_id, external_transaction_id),
    CONSTRAINT tx_provider_idem_uq UNIQUE (provider_id, idempotency_key)
);

CREATE UNIQUE INDEX one_opening_per_wallet ON wager_transactions (wallet_id) WHERE kind = 'OPENING';
CREATE UNIQUE INDEX one_processed_reversal_per_reference
    ON wager_transactions (provider_id, reference_external_transaction_id)
    WHERE kind IN ('REFUND','ROLLBACK') AND status = 'PROCESSED';

CREATE TABLE ledger_entries (
    id UUID PRIMARY KEY,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    transaction_id UUID NOT NULL REFERENCES wager_transactions(id),
    direction TEXT NOT NULL CHECK (direction IN ('DEBIT','CREDIT')),
    amount BIGINT NOT NULL CHECK (amount > 0),
    balance_before BIGINT NOT NULL CHECK (balance_before >= 0),
    balance_after BIGINT NOT NULL CHECK (balance_after >= 0),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT ledger_wallet_tx_uq UNIQUE (wallet_id, transaction_id),
    CONSTRAINT ledger_math_check CHECK (
        (direction = 'CREDIT' AND balance_after = balance_before + amount)
        OR (direction = 'DEBIT' AND balance_after = balance_before - amount)
    )
);

-- +goose StatementBegin
CREATE FUNCTION ledger_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ledger_entries is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER ledger_no_update_delete BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_immutable();
CREATE TRIGGER ledger_no_truncate BEFORE TRUNCATE ON ledger_entries
    FOR EACH STATEMENT EXECUTE FUNCTION ledger_immutable();

-- +goose Down
DROP TABLE ledger_entries;
DROP FUNCTION ledger_immutable();
DROP TABLE wager_transactions;
