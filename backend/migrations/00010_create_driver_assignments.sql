-- +goose Up

CREATE TABLE IF NOT EXISTS driver_assignments
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    company_id UUID NOT NULL,
    branch_id UUID NOT NULL,
    fleet_id UUID NOT NULL,

    driver_id UUID NOT NULL,
    vehicle_id UUID NOT NULL,

    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    unassigned_at TIMESTAMPTZ,

    notes TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_driver_assignments_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_assignments_branch
        FOREIGN KEY (branch_id)
        REFERENCES branches(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_assignments_fleet
        FOREIGN KEY (fleet_id)
        REFERENCES fleets(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_assignments_driver
        FOREIGN KEY (driver_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_driver_assignments_vehicle
        FOREIGN KEY (vehicle_id)
        REFERENCES vehicles(id)
        ON DELETE RESTRICT
);

-- A vehicle can have only one active assignment.
CREATE UNIQUE INDEX IF NOT EXISTS idx_driver_assignments_active_vehicle
ON driver_assignments(vehicle_id)
WHERE unassigned_at IS NULL;

-- A driver can have only one active assignment.
CREATE UNIQUE INDEX IF NOT EXISTS idx_driver_assignments_active_driver
ON driver_assignments(driver_id)
WHERE unassigned_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_driver_assignments_company
ON driver_assignments(company_id);

CREATE INDEX IF NOT EXISTS idx_driver_assignments_branch
ON driver_assignments(branch_id);

CREATE INDEX IF NOT EXISTS idx_driver_assignments_fleet
ON driver_assignments(fleet_id);

CREATE INDEX IF NOT EXISTS idx_driver_assignments_vehicle
ON driver_assignments(vehicle_id);

CREATE INDEX IF NOT EXISTS idx_driver_assignments_driver
ON driver_assignments(driver_id);

-- +goose Down

DROP TABLE IF EXISTS driver_assignments;