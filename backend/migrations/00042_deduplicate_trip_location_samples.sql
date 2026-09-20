-- +goose Up

-- Remove exact duplicate GPS observations already present.
-- Keep one row for each trip/driver/recorded_at combination.
DELETE FROM trip_locations a
USING trip_locations b
WHERE a.trip_id = b.trip_id
  AND a.driver_id = b.driver_id
  AND a.recorded_at = b.recorded_at
  AND a.id > b.id;

-- A single physical GPS observation must not be persisted more
-- than once for the same trip and driver. This protects trip-meter
-- evidence from duplicate browser/app submissions.
CREATE UNIQUE INDEX IF NOT EXISTS uq_trip_locations_trip_driver_recorded_at
ON trip_locations (
    trip_id,
    driver_id,
    recorded_at
);

-- +goose Down

DROP INDEX IF EXISTS uq_trip_locations_trip_driver_recorded_at;