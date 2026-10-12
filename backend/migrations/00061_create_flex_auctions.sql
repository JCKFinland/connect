-- +goose Up

-- CONNECT Flex negotiated-fare auction.
-- Standard dispatch tables remain unchanged.

CREATE TABLE flex_auctions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    ride_request_id UUID NOT NULL UNIQUE
        REFERENCES ride_requests(id) ON DELETE RESTRICT,

    passenger_offer_amount NUMERIC(10,2) NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'EUR',

    status VARCHAR(30) NOT NULL DEFAULT 'OPEN',

    selected_offer_id UUID,

    opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_flex_auction_amount
        CHECK (passenger_offer_amount > 0),

    CONSTRAINT chk_flex_auction_currency
        CHECK (currency ~ '^[A-Z]{3}$'),

    CONSTRAINT chk_flex_auction_status
        CHECK (status IN (
            'OPEN',
            'PENDING_AUTHORIZATION',
            'CONFIRMED',
            'EXPIRED',
            'CANCELLED',
            'FAILED'
        )),

    CONSTRAINT chk_flex_auction_expiry
        CHECK (expires_at > opened_at),

    CONSTRAINT chk_flex_auction_selected_offer
        CHECK (
            (status = 'OPEN' AND selected_offer_id IS NULL)
            OR
            (status IN ('PENDING_AUTHORIZATION', 'CONFIRMED')
                AND selected_offer_id IS NOT NULL)
            OR
            (status IN ('EXPIRED', 'CANCELLED', 'FAILED'))
        ),

    CONSTRAINT chk_flex_auction_resolution
        CHECK (
            (status IN ('OPEN', 'PENDING_AUTHORIZATION')
                AND resolved_at IS NULL)
            OR
            (status IN ('CONFIRMED', 'EXPIRED', 'CANCELLED', 'FAILED')
                AND resolved_at IS NOT NULL)
        )
);

CREATE INDEX idx_flex_auctions_status_expiry
    ON flex_auctions(status, expires_at);


-- PostgreSQL requires a unique referenced column set for the
-- composite assignment identity foreign key.
ALTER TABLE driver_assignments
    ADD CONSTRAINT uq_driver_assignments_flex_identity
    UNIQUE (
        id,
        driver_id,
        vehicle_id,
        company_id,
        branch_id,
        fleet_id
    );

CREATE TABLE flex_offers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    auction_id UUID NOT NULL
        REFERENCES flex_auctions(id) ON DELETE RESTRICT,

    assignment_id UUID NOT NULL
        REFERENCES driver_assignments(id) ON DELETE RESTRICT,

    -- Operational driver identity: users.id.
    driver_id UUID NOT NULL
        REFERENCES users(id) ON DELETE RESTRICT,

    vehicle_id UUID NOT NULL
        REFERENCES vehicles(id) ON DELETE RESTRICT,

    company_id UUID NOT NULL
        REFERENCES companies(id) ON DELETE RESTRICT,

    branch_id UUID NOT NULL
        REFERENCES branches(id) ON DELETE RESTRICT,

    fleet_id UUID NOT NULL
        REFERENCES fleets(id) ON DELETE RESTRICT,

    offered_amount NUMERIC(10,2) NOT NULL,

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    offered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_flex_offer_assignment_identity
        FOREIGN KEY (
            assignment_id,
            driver_id,
            vehicle_id,
            company_id,
            branch_id,
            fleet_id
        )
        REFERENCES driver_assignments (
            id,
            driver_id,
            vehicle_id,
            company_id,
            branch_id,
            fleet_id
        )
        ON DELETE RESTRICT,

    CONSTRAINT chk_flex_offer_amount
        CHECK (offered_amount > 0),

    CONSTRAINT chk_flex_offer_status
        CHECK (status IN (
            'PENDING',
            'SELECTED',
            'ACCEPTED',
            'REJECTED',
            'EXPIRED',
            'CANCELLED'
        )),

    CONSTRAINT chk_flex_offer_expiry
        CHECK (expires_at > offered_at),

    CONSTRAINT chk_flex_offer_response
        CHECK (
            (status = 'PENDING' AND responded_at IS NULL)
            OR status <> 'PENDING'
        )
);

-- Multiple drivers may compete, but each driver may have
-- only one currently pending bid in an auction.
CREATE UNIQUE INDEX uq_flex_offers_auction_driver
    ON flex_offers(auction_id, driver_id);

-- Support an auction-scoped winner reference.
ALTER TABLE flex_offers
    ADD CONSTRAINT uq_flex_offers_auction_identity
    UNIQUE (auction_id, id);

ALTER TABLE flex_auctions
    ADD CONSTRAINT fk_flex_auctions_selected_offer
    FOREIGN KEY (id, selected_offer_id)
    REFERENCES flex_offers(auction_id, id)
    ON DELETE RESTRICT;

-- A selected offer reserves the sole winner slot.
-- ACCEPTED represents a confirmed winner.
CREATE UNIQUE INDEX uq_flex_offers_single_winner
    ON flex_offers(auction_id)
    WHERE status IN ('SELECTED', 'ACCEPTED');

CREATE INDEX idx_flex_offers_auction
    ON flex_offers(auction_id, offered_at DESC);

CREATE INDEX idx_flex_offers_driver
    ON flex_offers(driver_id, offered_at DESC);

CREATE INDEX idx_flex_offers_assignment
    ON flex_offers(assignment_id);

-- +goose Down

-- Remove the circular foreign-key dependency before dropping tables.
ALTER TABLE flex_auctions
    DROP CONSTRAINT IF EXISTS fk_flex_auctions_selected_offer;

DROP TABLE IF EXISTS flex_offers;
DROP TABLE IF EXISTS flex_auctions;

ALTER TABLE driver_assignments
    DROP CONSTRAINT IF EXISTS uq_driver_assignments_flex_identity;
