-- +goose Up

-- CONNECT FLEX auction pricing provenance.
-- Historical auctions may predate this feature.

ALTER TABLE flex_auctions
    ADD COLUMN baseline_amount NUMERIC(10,2) NOT NULL,
    ADD COLUMN minimum_offer_amount NUMERIC(10,2) NOT NULL,
    ADD COLUMN maximum_offer_amount NUMERIC(10,2) NOT NULL,
    ADD COLUMN pricing_profile_id UUID NOT NULL,
    ADD COLUMN pricing_version TEXT NOT NULL;

ALTER TABLE flex_auctions
    ADD CONSTRAINT chk_flex_auction_pricing_provenance
    CHECK (
        baseline_amount > 0
        AND minimum_offer_amount > 0
        AND minimum_offer_amount <= baseline_amount
        AND baseline_amount <= maximum_offer_amount
        AND passenger_offer_amount >= minimum_offer_amount
        AND passenger_offer_amount <= maximum_offer_amount
        AND length(btrim(pricing_version)) > 0
    );

ALTER TABLE flex_auctions
    ADD CONSTRAINT fk_flex_auction_pricing_profile
    FOREIGN KEY (pricing_profile_id)
    REFERENCES fare_pricing_profiles(id)
    ON DELETE RESTRICT;

COMMENT ON COLUMN flex_auctions.baseline_amount IS
    'Canonical fare estimate at auction initiation, in EUR.';

COMMENT ON COLUMN flex_auctions.minimum_offer_amount IS
    'Minimum permitted passenger offer at auction initiation.';

COMMENT ON COLUMN flex_auctions.maximum_offer_amount IS
    'Maximum permitted passenger offer at auction initiation.';

COMMENT ON COLUMN flex_auctions.pricing_profile_id IS
    'Pricing profile used to calculate the auction baseline.';

COMMENT ON COLUMN flex_auctions.pricing_version IS
    'Pricing configuration version used at auction initiation.';

-- +goose Down

ALTER TABLE flex_auctions
    DROP CONSTRAINT IF EXISTS chk_flex_auction_pricing_provenance;

ALTER TABLE flex_auctions
    DROP CONSTRAINT IF EXISTS fk_flex_auction_pricing_profile;

ALTER TABLE flex_auctions
    DROP COLUMN IF EXISTS pricing_version,
    DROP COLUMN IF EXISTS pricing_profile_id,
    DROP COLUMN IF EXISTS maximum_offer_amount,
    DROP COLUMN IF EXISTS minimum_offer_amount,
    DROP COLUMN IF EXISTS baseline_amount;
