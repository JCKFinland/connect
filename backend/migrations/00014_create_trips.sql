-- +goose Up

CREATE TABLE IF NOT EXISTS trips
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    ride_request_id UUID NOT NULL,

    customer_id UUID NOT NULL,

    driver_id UUID NOT NULL,

    vehicle_id UUID NOT NULL,

    company_id UUID NOT NULL,

    branch_id UUID NOT NULL,

    fleet_id UUID NOT NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'ASSIGNED',

    estimated_distance_km NUMERIC(8,2),

    estimated_duration_minutes INTEGER,

    actual_distance_km NUMERIC(8,2),

    actual_duration_minutes INTEGER,

    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    driver_arrived_at TIMESTAMPTZ,

    pickup_at TIMESTAMPTZ,

    completed_at TIMESTAMPTZ,

    cancelled_at TIMESTAMPTZ,

    cancellation_reason TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_trips_request
        FOREIGN KEY (ride_request_id)
        REFERENCES ride_requests(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_trips_customer
        FOREIGN KEY (customer_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_trips_driver
        FOREIGN KEY (driver_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_trips_vehicle
        FOREIGN KEY (vehicle_id)
        REFERENCES vehicles(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_trips_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_trips_branch
        FOREIGN KEY (branch_id)
        REFERENCES branches(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_trips_fleet
        FOREIGN KEY (fleet_id)
        REFERENCES fleets(id)
        ON DELETE RESTRICT,

    CONSTRAINT chk_trip_status
        CHECK (
            status IN
            (
                'ASSIGNED',
                'DRIVER_EN_ROUTE',
                'DRIVER_ARRIVED',
                'IN_PROGRESS',
                'COMPLETED',
                'CANCELLED'
            )
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_trips_request
ON trips(ride_request_id);

CREATE INDEX IF NOT EXISTS idx_trips_customer
ON trips(customer_id);

CREATE INDEX IF NOT EXISTS idx_trips_driver
ON trips(driver_id);

CREATE INDEX IF NOT EXISTS idx_trips_vehicle
ON trips(vehicle_id);

CREATE INDEX IF NOT EXISTS idx_trips_company
ON trips(company_id);

CREATE INDEX IF NOT EXISTS idx_trips_status
ON trips(status);

CREATE INDEX IF NOT EXISTS idx_trips_created
ON trips(created_at);

-- +goose Down

DROP TABLE IF EXISTS trips;