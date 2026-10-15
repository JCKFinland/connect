-- +goose Up

-- CONNECT FLEX driver offer classification.
--
-- Existing offers were created exclusively through the
-- passenger-fare acceptance workflow and are therefore
-- classified as ACCEPT_FARE.
--
-- Offer classification must not be changed after creation.
-- The service layer enforces transactional auction eligibility,
-- fare equality for ACCEPT_FARE, and counter-offer boundaries.

ALTER TABLE flex_offers
    ADD COLUMN offer_type VARCHAR(30)
    NOT NULL DEFAULT 'ACCEPT_FARE';

ALTER TABLE flex_offers
    ADD CONSTRAINT chk_flex_offer_type
    CHECK (
        offer_type IN (
            'ACCEPT_FARE',
            'COUNTER_OFFER'
        )
    );

COMMENT ON COLUMN flex_offers.offer_type IS
    'Immutable driver offer classification: '
    'ACCEPT_FARE or COUNTER_OFFER.';

-- Prevent an existing offer from changing classification.
-- New offers remain insertable and other lifecycle fields
-- may still be updated by authorized transactional services.

-- +goose StatementBegin
CREATE FUNCTION prevent_flex_offer_type_change()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.offer_type IS DISTINCT FROM OLD.offer_type THEN
        RAISE EXCEPTION
            'FLEX offer type cannot be changed after creation'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_flex_offer_type_immutable
BEFORE UPDATE OF offer_type ON flex_offers
FOR EACH ROW
EXECUTE FUNCTION prevent_flex_offer_type_change();

-- +goose Down

DROP TRIGGER IF EXISTS trg_flex_offer_type_immutable
    ON flex_offers;

DROP FUNCTION IF EXISTS prevent_flex_offer_type_change();

ALTER TABLE flex_offers
    DROP CONSTRAINT IF EXISTS chk_flex_offer_type;

ALTER TABLE flex_offers
    DROP COLUMN IF EXISTS offer_type;
