-- +goose Up

CREATE TABLE IF NOT EXISTS ride_requests
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    customer_id UUID NOT NULL,

    pickup_address TEXT NOT NULL,
    pickup_latitude DECIMAL(10,8) NOT NULL,
    pickup_longitude DECIMAL(11,8) NOT NULL,

    destination_address TEXT NOT NULL,
    destination_latitude DECIMAL(10,8) NOT NULL,
    destination_longitude DECIMAL(11,8) NOT NULL,

    requested_vehicle_type VARCHAR(50) NOT NULL DEFAULT 'STANDARD',

    passenger_count INTEGER NOT NULL DEFAULT 1,

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    notes TEXT,

    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    expires_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_ride_requests_customer
        FOREIGN KEY (customer_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT chk_ride_request_status
        CHECK (
            status IN
            (
                'PENDING',
                'MATCHING',
                'ACCEPTED',
                'CANCELLED',
                'EXPIRED'
            )
        ),

    CONSTRAINT chk_passenger_count
        CHECK (passenger_count >= 1)
);

CREATE INDEX IF NOT EXISTS idx_ride_requests_customer
ON ride_requests(customer_id);

CREATE INDEX IF NOT EXISTS idx_ride_requests_status
ON ride_requests(status);

CREATE INDEX IF NOT EXISTS idx_ride_requests_requested_at
ON ride_requests(requested_at);

CREATE INDEX IF NOT EXISTS idx_ride_requests_pickup
ON ride_requests(pickup_latitude, pickup_longitude);

CREATE INDEX IF NOT EXISTS idx_ride_requests_destination
ON ride_requests(destination_latitude, destination_longitude);

-- +goose Down

DROP TABLE IF EXISTS ride_requests;