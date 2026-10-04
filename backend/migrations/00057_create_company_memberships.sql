-- +goose Up

-- ============================================================
-- CONNECT
-- Migration: 00057_create_company_memberships
-- Description: Canonical company membership authority
-- ============================================================

CREATE TABLE company_memberships (
    user_id UUID NOT NULL,
    company_id UUID NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, company_id),

    CONSTRAINT fk_company_memberships_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_company_memberships_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT
);

CREATE INDEX idx_company_memberships_company
    ON company_memberships(company_id);

-- +goose Down

DROP TABLE IF EXISTS company_memberships;
