-- +goose Up

CREATE TABLE flex_authorizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    auction_id UUID NOT NULL UNIQUE,
    offer_id UUID NOT NULL UNIQUE,
    passenger_id UUID NOT NULL,

    amount NUMERIC(10,2) NOT NULL,
    currency CHAR(3) NOT NULL,

    provider VARCHAR(30) NOT NULL DEFAULT 'STRIPE',
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,

    provider_authorization_id VARCHAR(255),

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    authorized_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_flex_authorizations_auction_offer
        FOREIGN KEY (auction_id, offer_id)
        REFERENCES flex_offers(auction_id, id)
        ON DELETE RESTRICT,

    CONSTRAINT fk_flex_authorizations_passenger
        FOREIGN KEY (passenger_id)
        REFERENCES users(id)
        ON DELETE RESTRICT,

    CONSTRAINT chk_flex_authorization_amount
        CHECK (amount > 0),

    CONSTRAINT chk_flex_authorization_currency
        CHECK (currency = 'EUR'),

    CONSTRAINT chk_flex_authorization_provider
        CHECK (provider = 'STRIPE'),

    CONSTRAINT chk_flex_authorization_status
        CHECK (status IN (
            'PENDING',
            'PROCESSING',
            'AUTHORIZED',
            'FAILED',
            'CANCELLED',
            'EXPIRED'
        )),

    CONSTRAINT chk_flex_authorization_authorized_at
        CHECK (
            (status = 'AUTHORIZED' AND authorized_at IS NOT NULL)
            OR
            (status <> 'AUTHORIZED')
        )
);

CREATE UNIQUE INDEX uq_flex_authorizations_provider_identity
    ON flex_authorizations(provider, provider_authorization_id)
    WHERE provider_authorization_id IS NOT NULL;

CREATE INDEX idx_flex_authorizations_status
    ON flex_authorizations(status);

CREATE INDEX idx_flex_authorizations_passenger
    ON flex_authorizations(passenger_id);

-- +goose Down

DROP TABLE IF EXISTS flex_authorizations;
