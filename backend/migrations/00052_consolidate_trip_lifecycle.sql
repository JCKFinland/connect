-- +goose Up

-- ============================================================
-- Consolidate trip lifecycle authority.
--
-- REQUESTED, SEARCHING_DRIVER, NO_DRIVER_AVAILABLE, and EXPIRED
-- belong to an obsolete trip lifecycle design. Current
-- pre-dispatch lifecycle is owned by ride_requests and
-- dispatch_offers. Operational trips use:
--
-- ASSIGNED
-- DRIVER_EN_ROUTE
-- DRIVER_ARRIVED
-- IN_PROGRESS
-- COMPLETED
-- CANCELLED
--
-- Fail closed if an environment still contains legacy trip
-- states. Such data requires explicit reconciliation before the
-- schema can be narrowed safely.
-- ============================================================

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM trips
        WHERE status IN (
            'REQUESTED',
            'SEARCHING_DRIVER',
            'NO_DRIVER_AVAILABLE',
            'EXPIRED'
        )
        LIMIT 1
    ) THEN
        RAISE EXCEPTION
            'cannot consolidate trip lifecycle: legacy trip statuses require explicit reconciliation';
    END IF;
END
$$;
-- +goose StatementEnd

DROP INDEX IF EXISTS idx_trips_searching_driver;

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

-- +goose Down

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

CREATE INDEX IF NOT EXISTS idx_trips_searching_driver
ON trips(status)
WHERE status = 'SEARCHING_DRIVER'
AND deleted_at IS NULL;
