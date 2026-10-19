-- +goose Up

-- Durable evidence that CONNECT may have submitted a Stripe
-- capture request. This marker must be persisted before
-- contacting the payment provider.

ALTER TABLE flex_settlements
    ADD COLUMN capture_requested_at TIMESTAMPTZ;

-- A capture request can only be recorded after the settlement
-- has entered processing and its financial records exist.

ALTER TABLE flex_settlements
    ADD CONSTRAINT chk_flex_capture_request_prepared
    CHECK (
        capture_requested_at IS NULL
        OR (
            payment_id IS NOT NULL
            AND capture_transaction_id IS NOT NULL
            AND status IN ('PROCESSING', 'CAPTURED')
        )
    );

-- Once capture execution is recorded, its timestamp is immutable.
-- The existing settlement trigger already validates ownership
-- and financial identity.

-- +goose StatementBegin
CREATE FUNCTION protect_flex_capture_requested_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND OLD.capture_requested_at IS NOT NULL
       AND NEW.capture_requested_at
           IS DISTINCT FROM OLD.capture_requested_at THEN
        RAISE EXCEPTION
            'FLEX capture request timestamp is immutable'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_protect_flex_capture_requested_at
BEFORE UPDATE ON flex_settlements
FOR EACH ROW
EXECUTE FUNCTION protect_flex_capture_requested_at();

-- +goose Down

DROP TRIGGER IF EXISTS trg_protect_flex_capture_requested_at
    ON flex_settlements;

DROP FUNCTION IF EXISTS protect_flex_capture_requested_at();

ALTER TABLE flex_settlements
    DROP CONSTRAINT IF EXISTS chk_flex_capture_request_prepared;

ALTER TABLE flex_settlements
    DROP COLUMN IF EXISTS capture_requested_at;
