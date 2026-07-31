-- +goose Up

CREATE TABLE IF NOT EXISTS branches
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    company_id UUID NOT NULL,

    code VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,

    email VARCHAR(255),
    phone VARCHAR(50),

    address_line1 VARCHAR(255),
    address_line2 VARCHAR(255),
    city VARCHAR(100),
    state VARCHAR(100),
    postal_code VARCHAR(30),

    latitude NUMERIC(9,6),
    longitude NUMERIC(9,6),

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT fk_branches_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT
);

-- Branch code must be unique within a company.
CREATE UNIQUE INDEX IF NOT EXISTS idx_branches_company_code
ON branches (company_id, code);

-- Useful indexes.
CREATE INDEX IF NOT EXISTS idx_branches_company
ON branches (company_id);

CREATE INDEX IF NOT EXISTS idx_branches_name
ON branches (name);

CREATE INDEX IF NOT EXISTS idx_branches_city
ON branches (city);

CREATE INDEX IF NOT EXISTS idx_branches_active
ON branches (is_active);

-- +goose Down

DROP TABLE IF EXISTS branches;