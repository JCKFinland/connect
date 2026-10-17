-- +goose Up

ALTER TABLE flex_authorizations
    ADD CONSTRAINT chk_flex_authorization_expiry
    CHECK (
        expires_at IS NOT NULL
        AND expires_at > created_at
    ) NOT VALID;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_flex_authorization_identity()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF ROW(
        NEW.auction_id,
        NEW.offer_id,
        NEW.passenger_id,
        NEW.amount,
        NEW.currency,
        NEW.provider,
        NEW.idempotency_key,
        NEW.created_at,
        NEW.expires_at
    ) IS DISTINCT FROM ROW(
        OLD.auction_id,
        OLD.offer_id,
        OLD.passenger_id,
        OLD.amount,
        OLD.currency,
        OLD.provider,
        OLD.idempotency_key,
        OLD.created_at,
        OLD.expires_at
    ) THEN
        RAISE EXCEPTION
            'FLEX authorization financial identity is immutable'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_protect_flex_authorization_identity
BEFORE UPDATE ON flex_authorizations
FOR EACH ROW
EXECUTE FUNCTION protect_flex_authorization_identity();

-- +goose Down

DROP TRIGGER IF EXISTS trg_protect_flex_authorization_identity
    ON flex_authorizations;

DROP FUNCTION IF EXISTS protect_flex_authorization_identity();

ALTER TABLE flex_authorizations
    DROP CONSTRAINT IF EXISTS chk_flex_authorization_expiry;
