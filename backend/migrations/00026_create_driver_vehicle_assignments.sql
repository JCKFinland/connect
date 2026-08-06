-- +goose Up

CREATE TABLE IF NOT EXISTS driver_vehicle_assignments
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

-- ------------------------------------------------------------------
-- Performance indexes
-- ------------------------------------------------------------------

CREATE INDEX IF NOT EXISTS idx_driver_vehicle_company
ON driver_vehicle_assignments(company_id);

CREATE INDEX IF NOT EXISTS idx_driver_vehicle_branch
ON driver_vehicle_assignments(branch_id);

CREATE INDEX IF NOT EXISTS idx_driver_vehicle_fleet
ON driver_vehicle_assignments(fleet_id);

CREATE INDEX IF NOT EXISTS idx_driver_vehicle_driver
ON driver_vehicle_assignments(driver_id);

CREATE INDEX IF NOT EXISTS idx_driver_vehicle_vehicle
ON driver_vehicle_assignments(vehicle_id);

CREATE INDEX IF NOT EXISTS idx_driver_vehicle_status
ON driver_vehicle_assignments(status);

CREATE INDEX IF NOT EXISTS idx_driver_vehicle_active
ON driver_vehicle_assignments(is_active);

-- ------------------------------------------------------------------
-- Business Rules
-- ------------------------------------------------------------------

-- Only one ACTIVE assignment per driver.
CREATE UNIQUE INDEX IF NOT EXISTS idx_driver_single_active_assignment
ON driver_vehicle_assignments(driver_id)
WHERE is_active = TRUE
AND deleted_at IS NULL;

-- Only one ACTIVE assignment per vehicle.
CREATE UNIQUE INDEX IF NOT EXISTS idx_vehicle_single_active_assignment
ON driver_vehicle_assignments(vehicle_id)
WHERE is_active = TRUE
AND deleted_at IS NULL;

-- +goose Down

DROP TABLE IF EXISTS driver_vehicle_assignments;