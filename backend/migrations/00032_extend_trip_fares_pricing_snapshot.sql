-- +goose Up

ALTER TABLE trip_fares
ADD COLUMN IF NOT EXISTS distance_rate_per_km NUMERIC(10,4) NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS time_rate_per_minute NUMERIC(10,4) NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS waiting_rate_per_minute NUMERIC(10,4) NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS charged_distance_meters BIGINT NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS charged_duration_seconds BIGINT NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS waiting_duration_seconds BIGINT NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS pricing_version VARCHAR(50) NOT NULL DEFAULT 'v1';

ALTER TABLE trip_fares
ADD CONSTRAINT chk_trip_fares_distance_rate
CHECK (distance_rate_per_km >= 0);

ALTER TABLE trip_fares
ADD CONSTRAINT chk_trip_fares_time_rate
CHECK (time_rate_per_minute >= 0);

ALTER TABLE trip_fares
ADD CONSTRAINT chk_trip_fares_waiting_rate
CHECK (waiting_rate_per_minute >= 0);

ALTER TABLE trip_fares
ADD CONSTRAINT chk_trip_fares_charged_distance
CHECK (charged_distance_meters >= 0);

ALTER TABLE trip_fares
ADD CONSTRAINT chk_trip_fares_charged_duration
CHECK (charged_duration_seconds >= 0);

ALTER TABLE trip_fares
ADD CONSTRAINT chk_trip_fares_waiting_duration
CHECK (waiting_duration_seconds >= 0);

-- +goose Down

ALTER TABLE trip_fares
DROP CONSTRAINT IF EXISTS chk_trip_fares_waiting_duration,
DROP CONSTRAINT IF EXISTS chk_trip_fares_charged_duration,
DROP CONSTRAINT IF EXISTS chk_trip_fares_charged_distance,
DROP CONSTRAINT IF EXISTS chk_trip_fares_waiting_rate,
DROP CONSTRAINT IF EXISTS chk_trip_fares_time_rate,
DROP CONSTRAINT IF EXISTS chk_trip_fares_distance_rate;

ALTER TABLE trip_fares
DROP COLUMN IF EXISTS pricing_version,
DROP COLUMN IF EXISTS waiting_duration_seconds,
DROP COLUMN IF EXISTS charged_duration_seconds,
DROP COLUMN IF EXISTS charged_distance_meters,
DROP COLUMN IF EXISTS waiting_rate_per_minute,
DROP COLUMN IF EXISTS time_rate_per_minute,
DROP COLUMN IF EXISTS distance_rate_per_km;