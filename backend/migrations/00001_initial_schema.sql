-- +goose Up

-- ============================================================
-- CONNECT
-- Migration: 00001_initial_schema
-- Description: Create initial users table
-- ============================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,

    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,

    phone VARCHAR(30),

    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_users_email
    ON users(email);

CREATE INDEX idx_users_phone
    ON users(phone);

CREATE INDEX idx_users_active
    ON users(is_active);

-- +goose Down

DROP TABLE IF EXISTS users;