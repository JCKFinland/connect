-- +goose Up

-- A platform user may have at most one non-deleted driver profile.
CREATE UNIQUE INDEX IF NOT EXISTS idx_drivers_unique_active_user
ON drivers (user_id)
WHERE deleted_at IS NULL;


-- +goose Down

DROP INDEX IF EXISTS idx_drivers_unique_active_user;