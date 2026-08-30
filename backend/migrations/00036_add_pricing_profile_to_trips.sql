-- +goose Up
ALTER TABLE trips
ADD COLUMN pricing_profile_id UUID NULL;

ALTER TABLE trips
ADD CONSTRAINT fk_trips_pricing_profile
FOREIGN KEY (pricing_profile_id)
REFERENCES fare_pricing_profiles(id)
ON DELETE RESTRICT;

CREATE INDEX idx_trips_pricing_profile_id
ON trips(pricing_profile_id);

-- +goose Down
DROP INDEX IF EXISTS idx_trips_pricing_profile_id;

ALTER TABLE trips
DROP CONSTRAINT IF EXISTS fk_trips_pricing_profile;

ALTER TABLE trips
DROP COLUMN IF EXISTS pricing_profile_id;