-- +goose Up

CREATE TABLE IF NOT EXISTS driver_statuses
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    driver_id UUID NOT NULL,

    status VARCHAR(30) NOT NULL,

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_driver_statuses_driver
        FOREIGN KEY (driver_id)
        REFERENCES users(id)
        ON DELETE CASCADE,

    CONSTRAINT chk_driver_status
        CHECK (
            status IN
            (
                'OFFLINE',
                'ONLINE',
                'AVAILABLE',
                'BUSY',
                'ON_TRIP',
                'ON_BREAK',
                'SUSPENDED'
            )
        )
);

-- One current status per driver.
CREATE UNIQUE INDEX IF NOT EXISTS idx_driver_statuses_driver
ON driver_statuses(driver_id);

-- Fast lookup by status (used by dispatch).
CREATE INDEX IF NOT EXISTS idx_driver_statuses_status
ON driver_statuses(status);

-- +goose Down

DROP TABLE IF EXISTS driver_statuses;