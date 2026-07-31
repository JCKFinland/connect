-- +goose Up

CREATE TABLE IF NOT EXISTS trip_fares
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    trip_id UUID NOT NULL UNIQUE,

    base_fare NUMERIC(10,2) NOT NULL DEFAULT 0,

    distance_fare NUMERIC(10,2) NOT NULL DEFAULT 0,

    time_fare NUMERIC(10,2) NOT NULL DEFAULT 0,

    waiting_fare NUMERIC(10,2) NOT NULL DEFAULT 0,

    booking_fee NUMERIC(10,2) NOT NULL DEFAULT 0,

    surge_multiplier NUMERIC(5,2) NOT NULL DEFAULT 1.00,

    surge_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    discount_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    tax_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    toll_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    parking_amount NUMERIC(10,2) NOT NULL DEFAULT 0,

    total_amount NUMERIC(10,2) NOT NULL,

    currency CHAR(3) NOT NULL DEFAULT 'EUR',

    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_trip_fares_trip
        FOREIGN KEY (trip_id)
        REFERENCES trips(id)
        ON DELETE CASCADE,

    CONSTRAINT chk_total_amount
        CHECK (total_amount >= 0),

    CONSTRAINT chk_surge_multiplier
        CHECK (surge_multiplier >= 1.00)
);

CREATE INDEX IF NOT EXISTS idx_trip_fares_trip
ON trip_fares(trip_id);

CREATE INDEX IF NOT EXISTS idx_trip_fares_currency
ON trip_fares(currency);

-- +goose Down

DROP TABLE IF EXISTS trip_fares;