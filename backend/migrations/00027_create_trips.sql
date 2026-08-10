-- +goose Up

-- ============================================================
-- Milestone 16
-- Extend the existing trips table.
--
-- The trips table was originally created in migration 00014.
-- This migration evolves that schema rather than recreating it.
-- ============================================================

-- ------------------------------------------------------------
-- Trip lifecycle
-- ------------------------------------------------------------

ALTER TABLE trips
DROP CONSTRAINT IF EXISTS chk_trip_status;

ALTER TABLE trips
ADD CONSTRAINT chk_trip_status
CHECK
(
    status IN
    (
        'REQUESTED',
        'SEARCHING_DRIVER',
        'ASSIGNED',
        'DRIVER_EN_ROUTE',
        'DRIVER_ARRIVED',
        'IN_PROGRESS',
        'COMPLETED',
        'CANCELLED',
        'NO_DRIVER_AVAILABLE',
        'EXPIRED'
    )
);

-- ------------------------------------------------------------
-- Scheduling
-- ------------------------------------------------------------

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMPTZ;

-- ------------------------------------------------------------
-- Pickup information
-- ------------------------------------------------------------

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS pickup_address TEXT;

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS pickup_latitude DOUBLE PRECISION;

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS pickup_longitude DOUBLE PRECISION;

-- ------------------------------------------------------------
-- Destination information
-- ------------------------------------------------------------

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS dropoff_address TEXT;

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS dropoff_latitude DOUBLE PRECISION;

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS dropoff_longitude DOUBLE PRECISION;

-- ------------------------------------------------------------
-- Passenger instructions
-- ------------------------------------------------------------

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS passenger_note TEXT;

-- ------------------------------------------------------------
-- Additional lifecycle timestamps
-- ------------------------------------------------------------

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS passenger_on_board_at TIMESTAMPTZ;

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;

-- ------------------------------------------------------------
-- Cancellation details
-- ------------------------------------------------------------

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS cancelled_by UUID;

ALTER TABLE trips
ADD CONSTRAINT fk_trips_cancelled_by
FOREIGN KEY (cancelled_by)
REFERENCES users(id)
ON DELETE RESTRICT;

-- ------------------------------------------------------------
-- Active / soft-delete state
-- ------------------------------------------------------------

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- ------------------------------------------------------------
-- Additional distance / duration metrics
-- ------------------------------------------------------------

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS estimated_distance_meters BIGINT;

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS estimated_duration_seconds BIGINT;

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS actual_distance_meters BIGINT;

ALTER TABLE trips
ADD COLUMN IF NOT EXISTS actual_duration_seconds BIGINT;

-- ------------------------------------------------------------
-- Indexes
-- ------------------------------------------------------------

CREATE INDEX IF NOT EXISTS idx_trips_branch
ON trips(branch_id);

CREATE INDEX IF NOT EXISTS idx_trips_fleet
ON trips(fleet_id);

CREATE INDEX IF NOT EXISTS idx_trips_scheduled_at
ON trips(scheduled_at);

CREATE INDEX IF NOT EXISTS idx_trips_active
ON trips(is_active);

CREATE INDEX IF NOT EXISTS idx_trips_deleted_at
ON trips(deleted_at);

CREATE INDEX IF NOT EXISTS idx_trips_searching_driver
ON trips(status)
WHERE status = 'SEARCHING_DRIVER'
AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_trips_active_driver
ON trips(driver_id)
WHERE is_active = TRUE
AND deleted_at IS NULL;

-- ------------------------------------------------------------
-- +goose Down
-- ------------------------------------------------------------

DROP INDEX IF EXISTS idx_trips_active_driver;

DROP INDEX IF EXISTS idx_trips_searching_driver;

DROP INDEX IF EXISTS idx_trips_deleted_at;

DROP INDEX IF EXISTS idx_trips_active;

DROP INDEX IF EXISTS idx_trips_scheduled_at;

DROP INDEX IF EXISTS idx_trips_fleet;

DROP INDEX IF EXISTS idx_trips_branch;

ALTER TABLE trips
DROP CONSTRAINT IF EXISTS fk_trips_cancelled_by;

ALTER TABLE trips
DROP COLUMN IF EXISTS actual_duration_seconds;

ALTER TABLE trips
DROP COLUMN IF EXISTS actual_distance_meters;

ALTER TABLE trips
DROP COLUMN IF EXISTS estimated_duration_seconds;

ALTER TABLE trips
DROP COLUMN IF EXISTS estimated_distance_meters;

ALTER TABLE trips
DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE trips
DROP COLUMN IF EXISTS is_active;

ALTER TABLE trips
DROP COLUMN IF EXISTS cancelled_by;

ALTER TABLE trips
DROP COLUMN IF EXISTS started_at;

ALTER TABLE trips
DROP COLUMN IF EXISTS passenger_on_board_at;

ALTER TABLE trips
DROP COLUMN IF EXISTS passenger_note;

ALTER TABLE trips
DROP COLUMN IF EXISTS dropoff_longitude;

ALTER TABLE trips
DROP COLUMN IF EXISTS dropoff_latitude;

ALTER TABLE trips
DROP COLUMN IF EXISTS dropoff_address;

ALTER TABLE trips
DROP COLUMN IF EXISTS pickup_longitude;

ALTER TABLE trips
DROP COLUMN IF EXISTS pickup_latitude;

ALTER TABLE trips
DROP COLUMN IF EXISTS pickup_address;

ALTER TABLE trips
DROP COLUMN IF EXISTS scheduled_at;

ALTER TABLE trips
DROP CONSTRAINT IF EXISTS chk_trip_status;

ALTER TABLE trips
ADD CONSTRAINT chk_trip_status
CHECK
(
    status IN
    (
        'ASSIGNED',
        'DRIVER_EN_ROUTE',
        'DRIVER_ARRIVED',
        'IN_PROGRESS',
        'COMPLETED',
        'CANCELLED'
    )
);