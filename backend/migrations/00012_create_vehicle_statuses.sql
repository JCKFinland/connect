-- +goose Up

CREATE TABLE IF NOT EXISTS vehicle_statuses
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    vehicle_id UUID NOT NULL,

    status VARCHAR(30) NOT NULL,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_vehicle_statuses_vehicle
        FOREIGN KEY (vehicle_id)
        REFERENCES vehicles(id)
        ON DELETE CASCADE,

    CONSTRAINT chk_vehicle_status
        CHECK (
            status IN
            (
                'AVAILABLE',
                'ON_TRIP',
                'MAINTENANCE',
                'OUT_OF_SERVICE',
                'INSPECTION',
                'CLEANING',
                'SUSPENDED'
            )
        )
);

-- One current status per vehicle.
CREATE UNIQUE INDEX IF NOT EXISTS idx_vehicle_statuses_vehicle
ON vehicle_statuses(vehicle_id);

-- Fast lookup by status for dispatch.
CREATE INDEX IF NOT EXISTS idx_vehicle_statuses_status
ON vehicle_statuses(status);

-- +goose Down

DROP TABLE IF EXISTS vehicle_statuses;