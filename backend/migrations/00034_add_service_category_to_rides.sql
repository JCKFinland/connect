-- +goose Up
-- +goose StatementBegin

ALTER TABLE ride_requests
ADD COLUMN service_category_id UUID;

ALTER TABLE ride_requests
ADD CONSTRAINT fk_ride_requests_service_category
FOREIGN KEY (service_category_id)
REFERENCES service_categories(id)
ON DELETE RESTRICT;

CREATE INDEX idx_ride_requests_service_category
ON ride_requests(service_category_id)
WHERE service_category_id IS NOT NULL;


ALTER TABLE trips
ADD COLUMN service_category_id UUID;

ALTER TABLE trips
ADD CONSTRAINT fk_trips_service_category
FOREIGN KEY (service_category_id)
REFERENCES service_categories(id)
ON DELETE RESTRICT;

CREATE INDEX idx_trips_service_category
ON trips(service_category_id)
WHERE service_category_id IS NOT NULL;


ALTER TABLE trip_fares
ADD COLUMN pricing_profile_id UUID;

ALTER TABLE trip_fares
ADD CONSTRAINT fk_trip_fares_pricing_profile
FOREIGN KEY (pricing_profile_id)
REFERENCES fare_pricing_profiles(id)
ON DELETE RESTRICT;

CREATE INDEX idx_trip_fares_pricing_profile
ON trip_fares(pricing_profile_id)
WHERE pricing_profile_id IS NOT NULL;

-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_trip_fares_pricing_profile;

ALTER TABLE trip_fares
DROP CONSTRAINT IF EXISTS fk_trip_fares_pricing_profile;

ALTER TABLE trip_fares
DROP COLUMN IF EXISTS pricing_profile_id;


DROP INDEX IF EXISTS idx_trips_service_category;

ALTER TABLE trips
DROP CONSTRAINT IF EXISTS fk_trips_service_category;

ALTER TABLE trips
DROP COLUMN IF EXISTS service_category_id;


DROP INDEX IF EXISTS idx_ride_requests_service_category;

ALTER TABLE ride_requests
DROP CONSTRAINT IF EXISTS fk_ride_requests_service_category;

ALTER TABLE ride_requests
DROP COLUMN IF EXISTS service_category_id;

-- +goose StatementEnd