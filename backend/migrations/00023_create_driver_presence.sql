-- +goose Up

CREATE TABLE IF NOT EXISTS driver_presence
(
    driver_id UUID PRIMARY KEY,

    company_id UUID NOT NULL,

    branch_id UUID,

    vehicle_id UUID,

    assignment_id UUID,

    is_online BOOLEAN NOT NULL DEFAULT FALSE,

    availability_status VARCHAR(30) NOT NULL DEFAULT 'OFFLINE',

    latitude DOUBLE PRECISION,

    longitude DOUBLE PRECISION,

    heading DOUBLE PRECISION,

    speed DOUBLE PRECISION,

    accuracy DOUBLE PRECISION,

    last_heartbeat_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_driver_presence_driver
        FOREIGN KEY (driver_id)
        REFERENCES users(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_driver_presence_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_driver_presence_branch
        FOREIGN KEY (branch_id)
        REFERENCES branches(id)
        ON DELETE SET NULL,

    CONSTRAINT fk_driver_presence_vehicle
        FOREIGN KEY (vehicle_id)
        REFERENCES vehicles(id)
        ON DELETE SET NULL,

    CONSTRAINT fk_driver_presence_assignment
        FOREIGN KEY (assignment_id)
        REFERENCES driver_assignments(id)
        ON DELETE SET NULL,

    CONSTRAINT chk_driver_availability
        CHECK
        (
            availability_status IN
            (
                'OFFLINE',
                'AVAILABLE',
                'BUSY',
                'BREAK',
                'OFF_DUTY',
                'SUSPENDED'
            )
        ),

    CONSTRAINT chk_latitude
        CHECK
        (
            latitude IS NULL
            OR
            (latitude >= -90 AND latitude <= 90)
        ),

    CONSTRAINT chk_longitude
        CHECK
        (
            longitude IS NULL
            OR
            (longitude >= -180 AND longitude <= 180)
        ),

    CONSTRAINT chk_heading
        CHECK
        (
            heading IS NULL
            OR
            (heading >= 0 AND heading <= 360)
        ),

    CONSTRAINT chk_speed
        CHECK
        (
            speed IS NULL
            OR
            speed >= 0
        ),

    CONSTRAINT chk_accuracy
        CHECK
        (
            accuracy IS NULL
            OR
            accuracy >= 0
        )
);

CREATE INDEX IF NOT EXISTS idx_driver_presence_company
ON driver_presence(company_id);

CREATE INDEX IF NOT EXISTS idx_driver_presence_branch
ON driver_presence(branch_id);

CREATE INDEX IF NOT EXISTS idx_driver_presence_vehicle
ON driver_presence(vehicle_id);

CREATE INDEX IF NOT EXISTS idx_driver_presence_status
ON driver_presence(availability_status);

CREATE INDEX IF NOT EXISTS idx_driver_presence_online
ON driver_presence(is_online);

CREATE INDEX IF NOT EXISTS idx_driver_presence_last_heartbeat
ON driver_presence(last_heartbeat_at);

CREATE INDEX IF NOT EXISTS idx_driver_presence_location
ON driver_presence(latitude, longitude);

-- +goose Down

DROP TABLE IF EXISTS driver_presence;