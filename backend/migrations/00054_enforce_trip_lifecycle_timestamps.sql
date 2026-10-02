-- +goose Up

-- ============================================================
-- Enforce consistency between trip lifecycle state and
-- lifecycle milestone timestamps.
--
-- Lifecycle milestones form an ordered history:
--
--   assigned_at
--       <= driver_arrived_at
--       <= started_at
--       <= completed_at / cancelled_at
--
-- DRIVER_EN_ROUTE has no dedicated timestamp column.
--
-- State shape:
--
--   ASSIGNED
--   DRIVER_EN_ROUTE
--       => no later lifecycle timestamps
--
--   DRIVER_ARRIVED
--       => driver_arrived_at is set
--       => no later lifecycle timestamps
--
--   IN_PROGRESS
--       => driver_arrived_at and started_at are set
--       => no terminal timestamp
--
--   COMPLETED
--       => driver_arrived_at, started_at, completed_at are set
--       => cancelled_at is not set
--
--   CANCELLED
--       => cancelled_at is set
--       => completed_at is not set
--       => any earlier milestones form a valid prefix
--
-- Soft-deleted trips are not exempt. Deletion changes
-- operational activity, not recorded lifecycle history.
--
-- Fail closed if existing data violates this invariant.
-- Historical lifecycle timestamps must not be fabricated merely
-- to make the constraint installable; violating rows require
-- explicit reconciliation from authoritative evidence.
-- ============================================================

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM trips
        WHERE NOT (
            (
                status IN (
                    'ASSIGNED',
                    'DRIVER_EN_ROUTE'
                )
                AND driver_arrived_at IS NULL
                AND started_at IS NULL
                AND completed_at IS NULL
                AND cancelled_at IS NULL
            )
            OR
            (
                status = 'DRIVER_ARRIVED'
                AND driver_arrived_at IS NOT NULL
                AND started_at IS NULL
                AND completed_at IS NULL
                AND cancelled_at IS NULL
            )
            OR
            (
                status = 'IN_PROGRESS'
                AND driver_arrived_at IS NOT NULL
                AND started_at IS NOT NULL
                AND completed_at IS NULL
                AND cancelled_at IS NULL
            )
            OR
            (
                status = 'COMPLETED'
                AND driver_arrived_at IS NOT NULL
                AND started_at IS NOT NULL
                AND completed_at IS NOT NULL
                AND cancelled_at IS NULL
            )
            OR
            (
                status = 'CANCELLED'
                AND cancelled_at IS NOT NULL
                AND completed_at IS NULL
                AND (
                    started_at IS NULL
                    OR driver_arrived_at IS NOT NULL
                )
            )
        )
        OR driver_arrived_at IS NOT NULL
           AND driver_arrived_at < assigned_at
        OR started_at IS NOT NULL
           AND (
               driver_arrived_at IS NULL
               OR started_at < driver_arrived_at
           )
        OR completed_at IS NOT NULL
           AND (
               started_at IS NULL
               OR completed_at < started_at
           )
        OR cancelled_at IS NOT NULL
           AND (
               cancelled_at < assigned_at
               OR (
                   driver_arrived_at IS NOT NULL
                   AND cancelled_at < driver_arrived_at
               )
               OR (
                   started_at IS NOT NULL
                   AND cancelled_at < started_at
               )
           )
        LIMIT 1
    ) THEN
        RAISE EXCEPTION
            'cannot enforce trip lifecycle timestamps: existing trip state requires explicit reconciliation';
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE trips
ADD CONSTRAINT chk_trip_lifecycle_timestamps
CHECK (
    (
        (
            status IN (
                'ASSIGNED',
                'DRIVER_EN_ROUTE'
            )
            AND driver_arrived_at IS NULL
            AND started_at IS NULL
            AND completed_at IS NULL
            AND cancelled_at IS NULL
        )
        OR
        (
            status = 'DRIVER_ARRIVED'
            AND driver_arrived_at IS NOT NULL
            AND started_at IS NULL
            AND completed_at IS NULL
            AND cancelled_at IS NULL
        )
        OR
        (
            status = 'IN_PROGRESS'
            AND driver_arrived_at IS NOT NULL
            AND started_at IS NOT NULL
            AND completed_at IS NULL
            AND cancelled_at IS NULL
        )
        OR
        (
            status = 'COMPLETED'
            AND driver_arrived_at IS NOT NULL
            AND started_at IS NOT NULL
            AND completed_at IS NOT NULL
            AND cancelled_at IS NULL
        )
        OR
        (
            status = 'CANCELLED'
            AND cancelled_at IS NOT NULL
            AND completed_at IS NULL
            AND (
                started_at IS NULL
                OR driver_arrived_at IS NOT NULL
            )
        )
    )
    AND (
        driver_arrived_at IS NULL
        OR driver_arrived_at >= assigned_at
    )
    AND (
        started_at IS NULL
        OR (
            driver_arrived_at IS NOT NULL
            AND started_at >= driver_arrived_at
        )
    )
    AND (
        completed_at IS NULL
        OR (
            started_at IS NOT NULL
            AND completed_at >= started_at
        )
    )
    AND (
        cancelled_at IS NULL
        OR (
            cancelled_at >= assigned_at
            AND (
                driver_arrived_at IS NULL
                OR cancelled_at >= driver_arrived_at
            )
            AND (
                started_at IS NULL
                OR cancelled_at >= started_at
            )
        )
    )
);

-- +goose Down

ALTER TABLE trips
DROP CONSTRAINT IF EXISTS chk_trip_lifecycle_timestamps;
