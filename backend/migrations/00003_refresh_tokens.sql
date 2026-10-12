-- +goose Up

-- Migration 00002 already creates refresh_tokens with the legacy
-- revoked BOOLEAN column. Convert that schema without deleting rows.

ALTER TABLE refresh_tokens
    ADD COLUMN revoked_at TIMESTAMPTZ;

-- Preserve the revoked state of any legacy tokens. The original
-- revocation time was not recorded, so use the migration time.
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE revoked = TRUE;

-- The application requires token hashes to identify unique tokens.
ALTER TABLE refresh_tokens
    ADD CONSTRAINT refresh_tokens_token_hash_key UNIQUE (token_hash);

-- Preserve the existing user index.
-- Replace the legacy expiry index with the application schema's name.
DROP INDEX IF EXISTS idx_refresh_tokens_expires;

CREATE INDEX idx_refresh_tokens_expiry
    ON refresh_tokens(expires_at);

CREATE INDEX idx_refresh_tokens_hash
    ON refresh_tokens(token_hash);

DROP INDEX IF EXISTS idx_refresh_tokens_revoked;

ALTER TABLE refresh_tokens
    DROP COLUMN revoked;

-- +goose Down

-- Reconstruct the legacy schema used by migration 00002.
ALTER TABLE refresh_tokens
    ADD COLUMN revoked BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE refresh_tokens
SET revoked = TRUE
WHERE revoked_at IS NOT NULL;

ALTER TABLE refresh_tokens
    DROP COLUMN revoked_at;

DROP INDEX IF EXISTS idx_refresh_tokens_hash;
DROP INDEX IF EXISTS idx_refresh_tokens_expiry;

CREATE INDEX idx_refresh_tokens_expires
    ON refresh_tokens(expires_at);

CREATE INDEX idx_refresh_tokens_revoked
    ON refresh_tokens(revoked);

ALTER TABLE refresh_tokens
    DROP CONSTRAINT IF EXISTS refresh_tokens_token_hash_key;
