-- +goose Up

CREATE TABLE IF NOT EXISTS payment_transactions
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    payment_id UUID NOT NULL,

    transaction_reference VARCHAR(100) NOT NULL UNIQUE,

    provider VARCHAR(50) NOT NULL,

    provider_transaction_id VARCHAR(255),

    idempotency_key VARCHAR(255),

    transaction_type VARCHAR(30) NOT NULL,

    status VARCHAR(30) NOT NULL,

    amount NUMERIC(10,2) NOT NULL,

    currency CHAR(3) NOT NULL DEFAULT 'EUR',

    gateway_request JSONB,

    gateway_response JSONB,

    processed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_payment_transactions_payment
        FOREIGN KEY (payment_id)
        REFERENCES payments(id)
        ON DELETE CASCADE,

    CONSTRAINT chk_transaction_type
        CHECK
        (
            transaction_type IN
            (
                'AUTHORIZE',
                'CAPTURE',
                'SALE',
                'REFUND',
                'VOID'
            )
        ),

    CONSTRAINT chk_transaction_status
        CHECK
        (
            status IN
            (
                'PENDING',
                'PROCESSING',
                'SUCCESS',
                'FAILED',
                'CANCELLED'
            )
        ),

    CONSTRAINT chk_transaction_amount
        CHECK (amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_payment_transactions_payment
ON payment_transactions(payment_id);

CREATE INDEX IF NOT EXISTS idx_payment_transactions_provider
ON payment_transactions(provider);

CREATE INDEX IF NOT EXISTS idx_payment_transactions_status
ON payment_transactions(status);

CREATE INDEX IF NOT EXISTS idx_payment_transactions_processed
ON payment_transactions(processed_at);

CREATE INDEX IF NOT EXISTS idx_payment_transactions_reference
ON payment_transactions(transaction_reference);

-- +goose Down

DROP TABLE IF EXISTS payment_transactions;