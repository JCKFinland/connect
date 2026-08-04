-- +goose Up

ALTER TABLE vehicles
RENAME COLUMN seat_capacity TO seating_capacity;

-- +goose Down

ALTER TABLE vehicles
RENAME COLUMN seating_capacity TO seat_capacity;