-- +goose Up

CREATE TABLE IF NOT EXISTS driver_earnings
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    trip_id UUID NOT NULL UNIQUE,

    driver_id UUID NOT NULL,

    company_id UUID NOT NULL,

    fare_id UUID NOT NULL,

    payment_id UUID,

    gross_amount NUMERIC(10,2) NOT NULL,

    commission_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    bonus_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    tip_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    adjustment_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    tax_withheld NUMERIC(10,2) NOT NULL DEFAULT 0,

    net_amount NUMERIC(10,2) NOT NULL,

    currency CHAR(3) NOT NULL DEFAULT 'EUR',

    settlement_status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    settled_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_driver_earnings_trip
        FOREIGN KEY (trip_id)
        REFERENCES trips(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_driver_earnings_driver
        FOREIGN KEY (driver_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_earnings_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_earnings_fare
        FOREIGN KEY (fare_id)
        REFERENCES trip_fares(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_earnings_payment
        FOREIGN KEY (payment_id)
        REFERENCES payments(id)
        ON DELETE SET NULL,

    CONSTRAINT chk_settlement_status
        CHECK
        (
            settlement_status IN
            (
                'PENDING',
                'READY',
                'PROCESSING',
                'PAID',
                'FAILED',
                'REVERSED'
            )
        ),

    CONSTRAINT chk_gross_amount
        CHECK (gross_amount >= 0),

    CONSTRAINT chk_net_amount
        CHECK (net_amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_driver_earnings_driver
ON driver_earnings(driver_id);

CREATE INDEX IF NOT EXISTS idx_driver_earnings_company
ON driver_earnings(company_id);

CREATE INDEX IF NOT EXISTS idx_driver_earnings_trip
ON driver_earnings(trip_id);

CREATE INDEX IF NOT EXISTS idx_driver_earnings_status
ON driver_earnings(settlement_status);

CREATE INDEX IF NOT EXISTS idx_driver_earnings_calculated
ON driver_earnings(calculated_at);

-- +goose Down

DROP TABLE IF EXISTS driver_earnings;