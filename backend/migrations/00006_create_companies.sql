-- +goose Up

CREATE TABLE IF NOT EXISTS companies
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name VARCHAR(255) NOT NULL,
    legal_name VARCHAR(255),
    business_id VARCHAR(100),

    email VARCHAR(255),
    phone VARCHAR(50),

    website VARCHAR(255),

    country_code CHAR(2) NOT NULL,
    timezone VARCHAR(100) NOT NULL,

    address_line1 VARCHAR(255),
    address_line2 VARCHAR(255),
    city VARCHAR(100),
    state VARCHAR(100),
    postal_code VARCHAR(30),

    logo_url TEXT,

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_companies_business_id
ON companies (business_id)
WHERE business_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_companies_name
ON companies (name);

CREATE INDEX IF NOT EXISTS idx_companies_active
ON companies (is_active);

CREATE INDEX IF NOT EXISTS idx_companies_country
ON companies (country_code);

-- +goose Down

DROP TABLE IF EXISTS companies;