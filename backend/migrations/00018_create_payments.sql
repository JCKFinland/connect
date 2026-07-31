-- +goose Up

CREATE TABLE IF NOT EXISTS payments
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    trip_id UUID NOT NULL UNIQUE,

    fare_id UUID NOT NULL UNIQUE,

    customer_id UUID NOT NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    payment_method VARCHAR(30) NOT NULL,

    amount NUMERIC(10,2) NOT NULL,

    currency CHAR(3) NOT NULL DEFAULT 'EUR',

    paid_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_payments_trip
        FOREIGN KEY (trip_id)
        REFERENCES trips(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_payments_fare
        FOREIGN KEY (fare_id)
        REFERENCES trip_fares(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_payments_customer
        FOREIGN KEY (customer_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT chk_payment_status
        CHECK (
            status IN
            (
                'PENDING',
                'PROCESSING',
                'AUTHORIZED',
                'PAID',
                'FAILED',
                'CANCELLED',
                'REFUNDED',
                'PARTIALLY_REFUNDED'
            )
        ),

    CONSTRAINT chk_payment_method
        CHECK (
            payment_method IN
            (
                'PI',
                'CARD',
                'CASH',
                'BANK_TRANSFER',
                'WALLET'
            )
        ),

    CONSTRAINT chk_payment_amount
        CHECK (amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_payments_customer
ON payments(customer_id);

CREATE INDEX IF NOT EXISTS idx_payments_status
ON payments(status);

CREATE INDEX IF NOT EXISTS idx_payments_method
ON payments(payment_method);

CREATE INDEX IF NOT EXISTS idx_payments_created
ON payments(created_at);

-- +goose Down

DROP TABLE IF EXISTS payments;