-- +goose Up

CREATE TABLE IF NOT EXISTS vehicles
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    company_id UUID NOT NULL,
    branch_id UUID NOT NULL,
    fleet_id UUID NOT NULL,

    registration_number VARCHAR(30) NOT NULL,
    vin VARCHAR(17),

    make VARCHAR(100) NOT NULL,
    model VARCHAR(100) NOT NULL,
    model_year INTEGER,

    color VARCHAR(50),

    vehicle_type VARCHAR(50) NOT NULL,
    fuel_type VARCHAR(30),

    seat_capacity INTEGER NOT NULL DEFAULT 4,

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT fk_vehicles_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_vehicles_branch
        FOREIGN KEY (branch_id)
        REFERENCES branches(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_vehicles_fleet
        FOREIGN KEY (fleet_id)
        REFERENCES fleets(id)
        ON DELETE RESTRICT
);

-- Registration numbers must be unique.
CREATE UNIQUE INDEX IF NOT EXISTS idx_vehicles_registration
ON vehicles(registration_number);

-- VINs should also be unique when present.
CREATE UNIQUE INDEX IF NOT EXISTS idx_vehicles_vin
ON vehicles(vin)
WHERE vin IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_vehicles_company
ON vehicles(company_id);

CREATE INDEX IF NOT EXISTS idx_vehicles_branch
ON vehicles(branch_id);

CREATE INDEX IF NOT EXISTS idx_vehicles_fleet
ON vehicles(fleet_id);

CREATE INDEX IF NOT EXISTS idx_vehicles_active
ON vehicles(is_active);

CREATE INDEX IF NOT EXISTS idx_vehicles_type
ON vehicles(vehicle_type);

-- +goose Down

DROP TABLE IF EXISTS vehicles;