-- +goose Up

CREATE TABLE IF NOT EXISTS driver_payouts
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    driver_id UUID NOT NULL,

    company_id UUID NOT NULL,

    payout_reference VARCHAR(100) NOT NULL UNIQUE,

    period_start TIMESTAMPTZ NOT NULL,

    period_end TIMESTAMPTZ NOT NULL,

    gross_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    deductions NUMERIC(10,2) NOT NULL DEFAULT 0,

    adjustments NUMERIC(10,2) NOT NULL DEFAULT 0,

    net_amount NUMERIC(10,2) NOT NULL,

    currency CHAR(3) NOT NULL DEFAULT 'EUR',

    payout_method VARCHAR(30) NOT NULL,

    payout_status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    provider VARCHAR(50),

    provider_reference VARCHAR(255),

    processed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_driver_payout_driver
        FOREIGN KEY (driver_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_payout_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT chk_payout_method
        CHECK
        (
            payout_method IN
            (
                'BANK_TRANSFER',
                'PI',
                'WALLET',
                'CASH'
            )
        ),

    CONSTRAINT chk_payout_status
        CHECK
        (
            payout_status IN
            (
                'PENDING',
                'PROCESSING',
                'PAID',
                'FAILED',
                'CANCELLED'
            )
        ),

    CONSTRAINT chk_net_amount
        CHECK (net_amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_driver_payouts_driver
ON driver_payouts(driver_id);

CREATE INDEX IF NOT EXISTS idx_driver_payouts_company
ON driver_payouts(company_id);

CREATE INDEX IF NOT EXISTS idx_driver_payouts_status
ON driver_payouts(payout_status);

CREATE INDEX IF NOT EXISTS idx_driver_payouts_period
ON driver_payouts(period_start, period_end);

CREATE INDEX IF NOT EXISTS idx_driver_payouts_reference
ON driver_payouts(payout_reference);

-- +goose Down

DROP TABLE IF EXISTS driver_payouts;