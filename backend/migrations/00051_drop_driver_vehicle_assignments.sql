-- +goose Up

-- driver_vehicle_assignments was a parallel assignment subsystem.
-- Operational assignment authority is driver_assignments.
--
-- Refuse to discard data silently. Any environment containing rows must
-- explicitly reconcile or archive them before this retirement migration runs.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM driver_vehicle_assignments
        LIMIT 1
    ) THEN
        RAISE EXCEPTION
            'cannot retire driver_vehicle_assignments: table contains data requiring explicit reconciliation';
    END IF;
END
$$;
-- +goose StatementEnd

DROP TABLE driver_vehicle_assignments;

-- +goose Down

CREATE TABLE driver_vehicle_assignments
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    company_id UUID NOT NULL,
    branch_id UUID NOT NULL,
    fleet_id UUID NOT NULL,

    driver_id UUID NOT NULL,
    vehicle_id UUID NOT NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'ACTIVE',

    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    released_at TIMESTAMPTZ,

    assigned_by UUID NOT NULL,

    notes TEXT,

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT fk_driver_vehicle_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_vehicle_branch
        FOREIGN KEY (branch_id)
        REFERENCES branches(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_vehicle_fleet
        FOREIGN KEY (fleet_id)
        REFERENCES fleets(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_vehicle_driver
        FOREIGN KEY (driver_id)
        REFERENCES drivers(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_vehicle_vehicle
        FOREIGN KEY (vehicle_id)
        REFERENCES vehicles(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_vehicle_assigned_by
        FOREIGN KEY (assigned_by)
        REFERENCES users(id)
        ON DELETE RESTRICT
);

CREATE INDEX idx_driver_vehicle_company
ON driver_vehicle_assignments(company_id);

CREATE INDEX idx_driver_vehicle_branch
ON driver_vehicle_assignments(branch_id);

CREATE INDEX idx_driver_vehicle_fleet
ON driver_vehicle_assignments(fleet_id);

CREATE INDEX idx_driver_vehicle_driver
ON driver_vehicle_assignments(driver_id);

CREATE INDEX idx_driver_vehicle_vehicle
ON driver_vehicle_assignments(vehicle_id);

CREATE INDEX idx_driver_vehicle_status
ON driver_vehicle_assignments(status);

CREATE INDEX idx_driver_vehicle_active
ON driver_vehicle_assignments(is_active);

CREATE UNIQUE INDEX idx_driver_single_active_assignment
ON driver_vehicle_assignments(driver_id)
WHERE is_active = TRUE
AND deleted_at IS NULL;

CREATE UNIQUE INDEX idx_vehicle_single_active_assignment
ON driver_vehicle_assignments(vehicle_id)
WHERE is_active = TRUE
AND deleted_at IS NULL;
