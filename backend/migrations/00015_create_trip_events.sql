-- +goose Up

CREATE TABLE IF NOT EXISTS trip_events
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    trip_id UUID NOT NULL,

    event_type VARCHAR(50) NOT NULL,

    performed_by_user_id UUID,

    latitude DECIMAL(10,8),
    longitude DECIMAL(11,8),

    metadata JSONB,

    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_trip_events_trip
        FOREIGN KEY (trip_id)
        REFERENCES trips(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_trip_events_user
        FOREIGN KEY (performed_by_user_id)
        REFERENCES users(id)
        ON DELETE SET NULL,

    CONSTRAINT chk_trip_event_type
        CHECK (
            event_type IN
            (
                'REQUEST_CREATED',
                'REQUEST_ACCEPTED',
                'DRIVER_EN_ROUTE',
                'DRIVER_ARRIVED',
                'PASSENGER_NO_SHOW',
                'TRIP_STARTED',
                'TRIP_COMPLETED',
                'TRIP_CANCELLED',
                'PAYMENT_COMPLETED',
                'PAYMENT_FAILED',
                'DRIVER_LOCATION_UPDATED',
                'PASSENGER_LOCATION_UPDATED'
            )
        )
);

CREATE INDEX IF NOT EXISTS idx_trip_events_trip
ON trip_events(trip_id);

CREATE INDEX IF NOT EXISTS idx_trip_events_type
ON trip_events(event_type);

CREATE INDEX IF NOT EXISTS idx_trip_events_time
ON trip_events(occurred_at);

CREATE INDEX IF NOT EXISTS idx_trip_events_trip_time
ON trip_events(trip_id, occurred_at);

-- +goose Down

DROP TABLE IF EXISTS trip_events;