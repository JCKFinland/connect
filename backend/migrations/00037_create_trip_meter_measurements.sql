-- +goose Up
-- +goose StatementBegin

CREATE TABLE trip_meter_measurements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    trip_id UUID NOT NULL,

    measurement_source VARCHAR(50) NOT NULL,
    algorithm_version VARCHAR(50) NOT NULL,

    distance_meters BIGINT NOT NULL,
    duration_seconds BIGINT NOT NULL,
    waiting_duration_seconds BIGINT NOT NULL,

    accepted_samples INTEGER NOT NULL DEFAULT 0,
    rejected_samples INTEGER NOT NULL DEFAULT 0,
    rejected_segments INTEGER NOT NULL DEFAULT 0,

    measured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_trip_meter_measurements_trip
        FOREIGN KEY (trip_id)
        REFERENCES trips(id)
        ON DELETE CASCADE,

    CONSTRAINT uq_trip_meter_measurements_trip
        UNIQUE (trip_id),

    CONSTRAINT chk_trip_meter_measurements_distance
        CHECK (distance_meters >= 0),

    CONSTRAINT chk_trip_meter_measurements_duration
        CHECK (duration_seconds >= 0),

    CONSTRAINT chk_trip_meter_measurements_waiting
        CHECK (waiting_duration_seconds >= 0),

    CONSTRAINT chk_trip_meter_measurements_accepted_samples
        CHECK (accepted_samples >= 0),

    CONSTRAINT chk_trip_meter_measurements_rejected_samples
        CHECK (rejected_samples >= 0),

    CONSTRAINT chk_trip_meter_measurements_rejected_segments
        CHECK (rejected_segments >= 0)
);

CREATE INDEX idx_trip_meter_measurements_source
    ON trip_meter_measurements(measurement_source);

CREATE INDEX idx_trip_meter_measurements_measured_at
    ON trip_meter_measurements(measured_at);

-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS trip_meter_measurements;

-- +goose StatementEnd