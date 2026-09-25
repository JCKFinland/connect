-- +goose Up

ALTER TABLE drivers
ADD COLUMN verified_at TIMESTAMPTZ,
ADD COLUMN verified_by_user_id UUID;

ALTER TABLE drivers
ADD CONSTRAINT fk_drivers_verified_by_user
FOREIGN KEY (verified_by_user_id)
REFERENCES users(id)
ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_drivers_verified_by_user
ON drivers(verified_by_user_id);

-- +goose Down

DROP INDEX IF EXISTS idx_drivers_verified_by_user;

ALTER TABLE drivers
DROP CONSTRAINT IF EXISTS fk_drivers_verified_by_user;

ALTER TABLE drivers
DROP COLUMN IF EXISTS verified_by_user_id,
DROP COLUMN IF EXISTS verified_at;