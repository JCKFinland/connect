-- +goose Up

-- ============================================================
-- Enforce driver assignment lifecycle chronology.
--
-- An assignment may be active:
--
--   unassigned_at IS NULL
--
-- or closed:
--
--   assigned_at <= unassigned_at
--
-- Historical assignment timestamps are lifecycle evidence.
-- Do not fabricate or rewrite them merely to make this
-- constraint installable. Fail closed if existing data violates
-- the invariant so that such rows require explicit reconciliation
-- from authoritative evidence.
-- ============================================================

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM driver_assignments
        WHERE unassigned_at IS NOT NULL
          AND unassigned_at < assigned_at
        LIMIT 1
    ) THEN
        RAISE EXCEPTION
            'cannot enforce driver assignment chronology: existing assignment history requires explicit reconciliation';
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE driver_assignments
ADD CONSTRAINT chk_driver_assignment_chronology
CHECK (
    unassigned_at IS NULL
    OR unassigned_at >= assigned_at
);

-- +goose Down

ALTER TABLE driver_assignments
DROP CONSTRAINT IF EXISTS chk_driver_assignment_chronology;
