-- +goose Up

CREATE TABLE flex_webhook_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    provider VARCHAR(30) NOT NULL DEFAULT 'STRIPE',
    provider_event_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,

    authorization_id UUID NOT NULL,
    provider_authorization_id VARCHAR(255) NOT NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'RECEIVED',

    attempt_count INTEGER NOT NULL DEFAULT 0,

    last_error TEXT,

    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_started_at TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_flex_webhook_events_authorization
        FOREIGN KEY (authorization_id)
        REFERENCES flex_authorizations(id)
        ON DELETE RESTRICT,

    CONSTRAINT uq_flex_webhook_provider_event
        UNIQUE (provider, provider_event_id),

    CONSTRAINT chk_flex_webhook_provider
        CHECK (provider = 'STRIPE'),

    CONSTRAINT chk_flex_webhook_event_id
        CHECK (BTRIM(provider_event_id) <> ''),

    CONSTRAINT chk_flex_webhook_event_type
        CHECK (BTRIM(event_type) <> ''),

    CONSTRAINT chk_flex_webhook_provider_authorization_id
        CHECK (BTRIM(provider_authorization_id) <> ''),

    CONSTRAINT chk_flex_webhook_status
        CHECK (
            status IN (
                'RECEIVED',
                'PROCESSING',
                'PROCESSED',
                'FAILED'
            )
        ),

    CONSTRAINT chk_flex_webhook_attempt_count
        CHECK (attempt_count >= 0),

    CONSTRAINT chk_flex_webhook_processed_at
        CHECK (
            (status = 'PROCESSED' AND processed_at IS NOT NULL)
            OR
            (status <> 'PROCESSED' AND processed_at IS NULL)
        )
);

CREATE INDEX idx_flex_webhook_events_authorization
    ON flex_webhook_events(authorization_id);

CREATE INDEX idx_flex_webhook_events_status
    ON flex_webhook_events(status, received_at);

CREATE INDEX idx_flex_webhook_events_provider_authorization
    ON flex_webhook_events(provider_authorization_id);

-- +goose Down

DROP TABLE IF EXISTS flex_webhook_events;
