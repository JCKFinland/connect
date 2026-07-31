-- +goose Up

CREATE TABLE IF NOT EXISTS trip_locations
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    trip_id UUID NOT NULL,

    driver_id UUID NOT NULL,

    latitude DECIMAL(10,8) NOT NULL,

    longitude DECIMAL(11,8) NOT NULL,

    altitude NUMERIC(8,2),

    speed_kmh NUMERIC(6,2),

    heading SMALLINT,

    accuracy_meters NUMERIC(6,2),

    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_trip_locations_trip
        FOREIGN KEY (trip_id)
        REFERENCES trips(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_trip_locations_driver
        FOREIGN KEY (driver_id)
        REFERENCES users(id)
        ON DELETE CASCADE,

    CONSTRAINT chk_latitude
        CHECK (
            latitude >= -90
            AND latitude <= 90
        ),

    CONSTRAINT chk_longitude
        CHECK (
            longitude >= -180
            AND longitude <= 180
        ),

    CONSTRAINT chk_speed
        CHECK (
            speed_kmh IS NULL
            OR speed_kmh >= 0
        ),

    CONSTRAINT chk_heading
        CHECK (
            heading IS NULL
            OR (
                heading >= 0
                AND heading <= 359
            )
        ),

    CONSTRAINT chk_accuracy
        CHECK (
            accuracy_meters IS NULL
            OR accuracy_meters >= 0
        )
);

CREATE INDEX IF NOT EXISTS idx_trip_locations_trip
ON trip_locations(trip_id);

CREATE INDEX IF NOT EXISTS idx_trip_locations_driver
ON trip_locations(driver_id);

CREATE INDEX IF NOT EXISTS idx_trip_locations_time
ON trip_locations(recorded_at);

CREATE INDEX IF NOT EXISTS idx_trip_locations_trip_time
ON trip_locations(trip_id, recorded_at);

CREATE INDEX IF NOT EXISTS idx_trip_locations_driver_time
ON trip_locations(driver_id, recorded_at);

-- +goose Down

DROP TABLE IF EXISTS trip_locations;