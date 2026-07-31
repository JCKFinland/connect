-- +goose Up

CREATE TABLE IF NOT EXISTS fleets
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    company_id UUID NOT NULL,
    branch_id UUID NOT NULL,

    code VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT fk_fleets_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_fleets_branch
        FOREIGN KEY (branch_id)
        REFERENCES branches(id)
        ON DELETE RESTRICT
);

-- Fleet code must be unique within a branch.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleets_branch_code
ON fleets(branch_id, code);

CREATE INDEX IF NOT EXISTS idx_fleets_company
ON fleets(company_id);

CREATE INDEX IF NOT EXISTS idx_fleets_branch
ON fleets(branch_id);

CREATE INDEX IF NOT EXISTS idx_fleets_name
ON fleets(name);

CREATE INDEX IF NOT EXISTS idx_fleets_active
ON fleets(is_active);

-- +goose Down

DROP TABLE IF EXISTS fleets;