-- +goose Up

-- ============================================================
-- CONNECT
-- Migration: 00002_create_auth_tables
-- Description: Authentication & Authorization (RBAC)
-- ============================================================

---------------------------------------------------------------
-- ROLES
---------------------------------------------------------------

CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

---------------------------------------------------------------
-- PERMISSIONS
---------------------------------------------------------------

CREATE TABLE permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name VARCHAR(150) NOT NULL UNIQUE,
    description TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

---------------------------------------------------------------
-- USER ROLES
---------------------------------------------------------------

CREATE TABLE user_roles (
    user_id UUID NOT NULL,
    role_id UUID NOT NULL,

    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, role_id),

    CONSTRAINT fk_user_roles_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_user_roles_role
        FOREIGN KEY (role_id)
        REFERENCES roles(id)
        ON DELETE CASCADE
);

---------------------------------------------------------------
-- ROLE PERMISSIONS
---------------------------------------------------------------

CREATE TABLE role_permissions (
    role_id UUID NOT NULL,
    permission_id UUID NOT NULL,

    PRIMARY KEY (role_id, permission_id),

    CONSTRAINT fk_role_permissions_role
        FOREIGN KEY (role_id)
        REFERENCES roles(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_role_permissions_permission
        FOREIGN KEY (permission_id)
        REFERENCES permissions(id)
        ON DELETE CASCADE
);

---------------------------------------------------------------
-- REFRESH TOKENS
---------------------------------------------------------------

CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL,

    token_hash TEXT NOT NULL,

    expires_at TIMESTAMPTZ NOT NULL,

    revoked BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_refresh_tokens_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE CASCADE
);

---------------------------------------------------------------
-- INDEXES
---------------------------------------------------------------

CREATE INDEX idx_refresh_tokens_user
    ON refresh_tokens(user_id);

CREATE INDEX idx_refresh_tokens_expires
    ON refresh_tokens(expires_at);

CREATE INDEX idx_refresh_tokens_revoked
    ON refresh_tokens(revoked);

---------------------------------------------------------------
-- DEFAULT ROLES
---------------------------------------------------------------

INSERT INTO roles (name, description) VALUES
('CUSTOMER', 'Customer using CONNECT'),
('DRIVER', 'Licensed taxi driver'),
('DISPATCHER', 'Manual dispatch operator'),
('FLEET_MANAGER', 'Fleet management'),
('COMPANY_ADMIN', 'Taxi company administrator'),
('SYSTEM_ADMIN', 'CONNECT system administrator');

-- +goose Down

DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;