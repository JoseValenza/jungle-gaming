CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id TEXT NOT NULL,
    currency CHAR(3) NOT NULL,
    balance_minor BIGINT NOT NULL DEFAULT 0 CHECK (balance_minor >= 0),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (player_id, currency)
);

CREATE TABLE wager_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id TEXT NOT NULL,
    external_transaction_id TEXT NOT NULL,
    player_id TEXT NOT NULL,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    round_id TEXT NOT NULL,
    game_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (
        kind IN ('OPENING', 'BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK')
    ),
    amount_minor BIGINT NOT NULL CHECK (amount_minor >= 0),
    currency CHAR(3) NOT NULL,
    status TEXT NOT NULL CHECK (
        status IN (
            'PENDING',
            'PENDING_REFERENCE',
            'PROCESSED',
            'REJECTED',
            'FAILED'
        )
    ),
    reference_external_transaction_id TEXT,
    idempotency_key TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    response_balance_minor BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider_id, external_transaction_id),
    UNIQUE (provider_id, idempotency_key)
);

CREATE INDEX idx_wager_transactions_wallet
    ON wager_transactions(wallet_id, created_at DESC);

CREATE TABLE ledger_entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    transaction_id UUID NOT NULL REFERENCES wager_transactions(id),
    direction TEXT NOT NULL CHECK (direction IN ('CREDIT', 'DEBIT')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    balance_before_minor BIGINT NOT NULL CHECK (balance_before_minor >= 0),
    balance_after_minor BIGINT NOT NULL CHECK (balance_after_minor >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (wallet_id, transaction_id)
);

CREATE INDEX idx_ledger_wallet_created
    ON ledger_entries(wallet_id, created_at DESC, id DESC);

CREATE TABLE idempotency_records (
    provider_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    transaction_id UUID NOT NULL REFERENCES wager_transactions(id),
    response_status INTEGER,
    response_body JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider_id, idempotency_key)
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    causation_id TEXT,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_outbox_unpublished
    ON outbox_events(created_at)
    WHERE published_at IS NULL;

CREATE TABLE inbox_messages (
    consumer_name TEXT NOT NULL,
    message_id TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (consumer_name, message_id)
);
