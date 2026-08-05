-- +goose Up

CREATE TABLE IF NOT EXISTS drivers
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL,
    company_id UUID NOT NULL,
    branch_id UUID NOT NULL,

    driver_number VARCHAR(50) NOT NULL,

    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,

    phone VARCHAR(30) NOT NULL,
    email VARCHAR(255),

    taxi_driver_license_number VARCHAR(100) NOT NULL,

    driving_license_number VARCHAR(100) NOT NULL,
    driving_license_expiry DATE,

    hire_date DATE,

    status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',

    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT fk_drivers_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_drivers_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_drivers_branch
        FOREIGN KEY (branch_id)
        REFERENCES branches(id)
        ON DELETE RESTRICT
);

-- Driver number must be globally unique.
CREATE UNIQUE INDEX IF NOT EXISTS idx_drivers_driver_number
ON drivers(driver_number);

-- Taxi Driver Permit number must be unique.
CREATE UNIQUE INDEX IF NOT EXISTS idx_drivers_taxi_license
ON drivers(taxi_driver_license_number);

-- Driving licence number should also be unique.
CREATE UNIQUE INDEX IF NOT EXISTS idx_drivers_driving_license
ON drivers(driving_license_number);

CREATE INDEX IF NOT EXISTS idx_drivers_company
ON drivers(company_id);

CREATE INDEX IF NOT EXISTS idx_drivers_branch
ON drivers(branch_id);

CREATE INDEX IF NOT EXISTS idx_drivers_status
ON drivers(status);

CREATE INDEX IF NOT EXISTS idx_drivers_active
ON drivers(is_active);

-- +goose Down

DROP TABLE IF EXISTS drivers;