-- +goose Up

-- ============================================================
-- Dispatch Offers
--
-- Represents an offer of a ride request to a specific driver.
--
-- A ride request may have multiple historical offers as drivers
-- reject or offers expire, but only one PENDING offer may exist
-- for the same ride request at a time.
--
-- A driver may also have only one PENDING offer at a time.
-- ============================================================

CREATE TABLE IF NOT EXISTS dispatch_offers
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- --------------------------------------------------------
    -- Ride being offered
    -- --------------------------------------------------------

    ride_request_id UUID NOT NULL,

    -- --------------------------------------------------------
    -- Selected operational driver and vehicle
    -- --------------------------------------------------------

    driver_id UUID NOT NULL,
    vehicle_id UUID NOT NULL,

    -- --------------------------------------------------------
    -- Tenant / fleet ownership snapshot
    -- --------------------------------------------------------

    company_id UUID NOT NULL,
    branch_id UUID NOT NULL,
    fleet_id UUID NOT NULL,

    -- --------------------------------------------------------
    -- Offer lifecycle
    -- --------------------------------------------------------

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    offered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    expires_at TIMESTAMPTZ NOT NULL,

    responded_at TIMESTAMPTZ,

    -- Driver-supplied or system-generated rejection information.
    rejection_reason TEXT,

    -- --------------------------------------------------------
    -- Audit actor
    --
    -- NULL is permitted for system-generated automatic dispatch.
    -- --------------------------------------------------------

    created_by UUID,

    -- --------------------------------------------------------
    -- Standard timestamps
    -- --------------------------------------------------------

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- --------------------------------------------------------
    -- Foreign keys
    -- --------------------------------------------------------

    CONSTRAINT fk_dispatch_offers_ride_request
        FOREIGN KEY (ride_request_id)
        REFERENCES ride_requests(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_dispatch_offers_driver
        FOREIGN KEY (driver_id)
        REFERENCES drivers(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_dispatch_offers_vehicle
        FOREIGN KEY (vehicle_id)
        REFERENCES vehicles(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_dispatch_offers_company
        FOREIGN KEY (company_id)
        REFERENCES companies(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_dispatch_offers_branch
        FOREIGN KEY (branch_id)
        REFERENCES branches(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_dispatch_offers_fleet
        FOREIGN KEY (fleet_id)
        REFERENCES fleets(id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_dispatch_offers_created_by
        FOREIGN KEY (created_by)
        REFERENCES users(id)
        ON DELETE SET NULL,

    -- --------------------------------------------------------
    -- Lifecycle validation
    -- --------------------------------------------------------

    CONSTRAINT chk_dispatch_offer_status
        CHECK (
            status IN
            (
                'PENDING',
                'ACCEPTED',
                'REJECTED',
                'EXPIRED',
                'CANCELLED'
            )
        ),

    -- Every offer must expire after it was issued.
    CONSTRAINT chk_dispatch_offer_expiry
        CHECK (
            expires_at > offered_at
        ),

    -- A response timestamp is expected only after a response/
    -- terminal decision has been made.
    CONSTRAINT chk_dispatch_offer_response
        CHECK (
            (
                status = 'PENDING'
                AND responded_at IS NULL
            )
            OR
            (
                status IN
                (
                    'ACCEPTED',
                    'REJECTED',
                    'EXPIRED',
                    'CANCELLED'
                )
            )
        )
);

-- ============================================================
-- Query indexes
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_dispatch_offers_ride_request
ON dispatch_offers(ride_request_id);

CREATE INDEX IF NOT EXISTS idx_dispatch_offers_driver
ON dispatch_offers(driver_id);

CREATE INDEX IF NOT EXISTS idx_dispatch_offers_vehicle
ON dispatch_offers(vehicle_id);

CREATE INDEX IF NOT EXISTS idx_dispatch_offers_company
ON dispatch_offers(company_id);

CREATE INDEX IF NOT EXISTS idx_dispatch_offers_branch
ON dispatch_offers(branch_id);

CREATE INDEX IF NOT EXISTS idx_dispatch_offers_status
ON dispatch_offers(status);

CREATE INDEX IF NOT EXISTS idx_dispatch_offers_expires_at
ON dispatch_offers(expires_at);

CREATE INDEX IF NOT EXISTS idx_dispatch_offers_request_time
ON dispatch_offers(
    ride_request_id,
    offered_at DESC
);

CREATE INDEX IF NOT EXISTS idx_dispatch_offers_driver_time
ON dispatch_offers(
    driver_id,
    offered_at DESC
);

-- ============================================================
-- Concurrency / business rules
-- ============================================================

-- A ride request can have many historical offers, but only one
-- currently active PENDING offer.
CREATE UNIQUE INDEX IF NOT EXISTS idx_dispatch_offers_single_pending_request
ON dispatch_offers(ride_request_id)
WHERE status = 'PENDING';

-- A driver may only have one active PENDING ride offer.
CREATE UNIQUE INDEX IF NOT EXISTS idx_dispatch_offers_single_pending_driver
ON dispatch_offers(driver_id)
WHERE status = 'PENDING';

-- ============================================================
-- +goose Down
-- ============================================================

DROP TABLE IF EXISTS dispatch_offers;