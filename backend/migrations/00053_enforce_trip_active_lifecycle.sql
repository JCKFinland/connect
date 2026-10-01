-- +goose Up

-- ============================================================
-- Enforce consistency between trip lifecycle state and
-- operational activity.
--
-- For live (non-deleted) trips:
--
--   ASSIGNED
--   DRIVER_EN_ROUTE
--   DRIVER_ARRIVED
--   IN_PROGRESS
--       => is_active = TRUE
--
--   COMPLETED
--   CANCELLED
--       => is_active = FALSE
--
-- Soft-deleted trips are intentionally exempt. Deletion removes
-- a trip from operational activity without rewriting its last
-- lifecycle status.
--
-- Fail closed if existing live data violates this invariant.
-- Such rows require explicit reconciliation before the database
-- constraint can be installed safely.
-- ============================================================

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM trips
        WHERE deleted_at IS NULL
          AND (
                (
                    status IN (
                        'ASSIGNED',
                        'DRIVER_EN_ROUTE',
                        'DRIVER_ARRIVED',
                        'IN_PROGRESS'
                    )
                    AND is_active IS DISTINCT FROM TRUE
                )
                OR
                (
                    status IN (
                        'COMPLETED',
                        'CANCELLED'
                    )
                    AND is_active IS DISTINCT FROM FALSE
                )
          )
        LIMIT 1
    ) THEN
        RAISE EXCEPTION
            'cannot enforce trip active lifecycle: live trip state requires explicit reconciliation';
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE trips
ADD CONSTRAINT chk_trip_active_lifecycle
CHECK (
    deleted_at IS NOT NULL
    OR (
        status IN (
            'ASSIGNED',
            'DRIVER_EN_ROUTE',
            'DRIVER_ARRIVED',
            'IN_PROGRESS'
        )
        AND is_active IS TRUE
    )
    OR (
        status IN (
            'COMPLETED',
            'CANCELLED'
        )
        AND is_active IS FALSE
    )
);

-- +goose Down

ALTER TABLE trips
DROP CONSTRAINT IF EXISTS chk_trip_active_lifecycle;
