-- +goose Up

ALTER TABLE vehicles
ADD COLUMN IF NOT EXISTS fuel_type VARCHAR(30);

UPDATE vehicles
SET fuel_type = 'PETROL'
WHERE fuel_type IS NULL
   OR BTRIM(fuel_type) = '';

ALTER TABLE vehicles
ALTER COLUMN fuel_type SET DEFAULT 'PETROL';

ALTER TABLE vehicles
ALTER COLUMN fuel_type SET NOT NULL;

ALTER TABLE vehicles
ADD CONSTRAINT chk_vehicles_fuel_type
CHECK (fuel_type IN ('EV', 'HYBRID', 'PETROL', 'DIESEL'));

CREATE INDEX IF NOT EXISTS idx_vehicles_fuel_type
ON vehicles(fuel_type);

-- +goose Down

DROP INDEX IF EXISTS idx_vehicles_fuel_type;

ALTER TABLE vehicles
DROP CONSTRAINT IF EXISTS chk_vehicles_fuel_type;

ALTER TABLE vehicles
ALTER COLUMN fuel_type DROP NOT NULL;

ALTER TABLE vehicles
ALTER COLUMN fuel_type DROP DEFAULT;