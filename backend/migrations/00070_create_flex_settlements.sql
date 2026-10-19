-- +goose Up

-- CONNECT FLEX settlement coordination.
-- payments and payment_transactions remain authoritative financial records.

CREATE TABLE flex_settlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    trip_id UUID NOT NULL UNIQUE
        REFERENCES trips(id) ON DELETE RESTRICT,

    authorization_id UUID NOT NULL UNIQUE
        REFERENCES flex_authorizations(id) ON DELETE RESTRICT,

    payment_id UUID UNIQUE
        REFERENCES payments(id) ON DELETE RESTRICT,

    capture_transaction_id UUID UNIQUE
        REFERENCES payment_transactions(id) ON DELETE RESTRICT,

    amount NUMERIC(10,2) NOT NULL,
    currency CHAR(3) NOT NULL,

    provider VARCHAR(30) NOT NULL DEFAULT 'STRIPE',
    provider_authorization_id VARCHAR(255) NOT NULL,
    capture_idempotency_key VARCHAR(255) NOT NULL UNIQUE,

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    provider_capture_id VARCHAR(255),

    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,

    captured_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_flex_settlement_amount
        CHECK (amount > 0),

    CONSTRAINT chk_flex_settlement_currency
        CHECK (currency = 'EUR'),

    CONSTRAINT chk_flex_settlement_provider
        CHECK (provider = 'STRIPE'),

    CONSTRAINT chk_flex_settlement_status
        CHECK (
            status IN (
                'PENDING',
                'PROCESSING',
                'CAPTURED',
                'CANCELLED'
            )
        ),

    CONSTRAINT chk_flex_settlement_attempt_count
        CHECK (attempt_count >= 0),

    CONSTRAINT chk_flex_settlement_capture_timestamp
        CHECK (
            (status = 'CAPTURED' AND captured_at IS NOT NULL)
            OR
            (status <> 'CAPTURED' AND captured_at IS NULL)
        ),

    CONSTRAINT chk_flex_settlement_capture_evidence
        CHECK (
            status <> 'CAPTURED'
            OR (
                payment_id IS NOT NULL
                AND capture_transaction_id IS NOT NULL
                AND provider_capture_id IS NOT NULL
                AND BTRIM(provider_capture_id) <> ''
                AND provider_capture_id = provider_authorization_id
            )
        ),

    CONSTRAINT chk_flex_settlement_provider_identity
        CHECK (
            BTRIM(provider_authorization_id) <> ''
            AND BTRIM(capture_idempotency_key) <> ''
        ),

    CONSTRAINT chk_flex_settlement_capture_link
        CHECK (
            capture_transaction_id IS NULL
            OR payment_id IS NOT NULL
        )
);

CREATE UNIQUE INDEX uq_flex_settlements_provider_capture
    ON flex_settlements(provider, provider_capture_id)
    WHERE provider_capture_id IS NOT NULL;

CREATE INDEX idx_flex_settlements_status
    ON flex_settlements(status, created_at);

-- +goose StatementBegin
CREATE FUNCTION validate_flex_settlement_identity()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    matching_identity BOOLEAN;
    matching_capture BOOLEAN;
BEGIN
    SELECT EXISTS (
        SELECT 1
        FROM trips t
        JOIN ride_requests rr
            ON rr.id = t.ride_request_id
        JOIN flex_auctions a
            ON a.ride_request_id = rr.id
        JOIN flex_authorizations fa
            ON fa.auction_id = a.id
        JOIN flex_offers fo
            ON fo.id = fa.offer_id
           AND fo.auction_id = a.id
        WHERE t.id = NEW.trip_id
          AND t.status = 'COMPLETED'
          AND t.deleted_at IS NULL
          AND fa.id = NEW.authorization_id
          AND rr.matching_mode = 'FLEX'
          AND t.customer_id = rr.customer_id
          AND fa.passenger_id = t.customer_id
          AND a.status = 'CONFIRMED'
          AND fo.status = 'ACCEPTED'
          AND t.driver_id = fo.driver_id
          AND t.vehicle_id = fo.vehicle_id
          AND t.company_id = fo.company_id
          AND t.branch_id = fo.branch_id
          AND t.fleet_id = fo.fleet_id
          AND fa.status = 'AUTHORIZED'
          AND fa.provider = NEW.provider
          AND fa.provider_authorization_id =
              NEW.provider_authorization_id
          AND fa.amount = NEW.amount
          AND fa.currency = NEW.currency
          AND fo.offered_amount = NEW.amount
          AND a.selected_offer_id = fo.id
          AND EXISTS (
              SELECT 1
              FROM trip_fares tf
              WHERE tf.trip_id = t.id
                AND tf.total_amount = NEW.amount
                AND tf.currency = NEW.currency
          )
          AND (
              NEW.payment_id IS NULL
              OR EXISTS (
                  SELECT 1
                  FROM payments p
                  JOIN trip_fares tf
                    ON tf.id = p.fare_id
                  WHERE p.id = NEW.payment_id
                    AND p.trip_id = t.id
                    AND p.customer_id = t.customer_id
                    AND p.amount = NEW.amount
                    AND p.currency = NEW.currency
                    AND tf.trip_id = t.id
                    AND tf.total_amount = NEW.amount
                    AND tf.currency = NEW.currency
              )
          )
    )
    INTO matching_identity;

    IF NOT matching_identity THEN
        RAISE EXCEPTION
            'FLEX settlement financial or ownership identity mismatch'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.capture_transaction_id IS NOT NULL THEN
        SELECT EXISTS (
            SELECT 1
            FROM payment_transactions pt
            JOIN payments p
              ON p.id = pt.payment_id
            WHERE pt.id = NEW.capture_transaction_id
              AND pt.payment_id = NEW.payment_id
              AND pt.provider = NEW.provider
              AND pt.transaction_type = 'CAPTURE'
              AND pt.idempotency_key =
                  NEW.capture_idempotency_key
              AND pt.amount = NEW.amount
              AND pt.currency = NEW.currency
              AND p.trip_id = NEW.trip_id
              AND p.amount = NEW.amount
              AND p.currency = NEW.currency
              AND (
                  NEW.status <> 'CAPTURED'
                  OR (
                      pt.status = 'SUCCESS'
                      AND p.status = 'PAID'
                      AND pt.provider_transaction_id =
                          NEW.provider_capture_id
                  )
              )
        )
        INTO matching_capture;

        IF NOT matching_capture THEN
            RAISE EXCEPTION
                'FLEX settlement capture transaction mismatch'
                USING ERRCODE = '23514';
        END IF;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF ROW(
            NEW.trip_id,
            NEW.authorization_id,
            NEW.amount,
            NEW.currency,
            NEW.provider,
            NEW.provider_authorization_id,
            NEW.capture_idempotency_key
        ) IS DISTINCT FROM ROW(
            OLD.trip_id,
            OLD.authorization_id,
            OLD.amount,
            OLD.currency,
            OLD.provider,
            OLD.provider_authorization_id,
            OLD.capture_idempotency_key
        ) THEN
            RAISE EXCEPTION
                'FLEX settlement financial identity is immutable'
                USING ERRCODE = '23514';
        END IF;

        IF OLD.payment_id IS NOT NULL
           AND NEW.payment_id IS DISTINCT FROM OLD.payment_id THEN
            RAISE EXCEPTION
                'FLEX settlement payment identity cannot change'
                USING ERRCODE = '23514';
        END IF;

        IF OLD.capture_transaction_id IS NOT NULL
           AND NEW.capture_transaction_id
               IS DISTINCT FROM OLD.capture_transaction_id THEN
            RAISE EXCEPTION
                'FLEX capture transaction identity cannot change'
                USING ERRCODE = '23514';
        END IF;

        IF OLD.provider_capture_id IS NOT NULL
           AND NEW.provider_capture_id
               IS DISTINCT FROM OLD.provider_capture_id THEN
            RAISE EXCEPTION
                'FLEX provider capture identity is immutable'
                USING ERRCODE = '23514';
        END IF;

        IF OLD.captured_at IS NOT NULL
           AND NEW.captured_at IS DISTINCT FROM OLD.captured_at THEN
            RAISE EXCEPTION
                'FLEX capture timestamp is immutable'
                USING ERRCODE = '23514';
        END IF;

        IF NEW.attempt_count < OLD.attempt_count THEN
            RAISE EXCEPTION
                'FLEX capture attempt count cannot decrease'
                USING ERRCODE = '23514';
        END IF;

        IF NEW.status IS DISTINCT FROM OLD.status THEN
            IF NOT (
                (OLD.status = 'PENDING'
                    AND NEW.status IN ('PROCESSING', 'CANCELLED'))
                OR
                (OLD.status = 'PROCESSING'
                    AND NEW.status IN ('PENDING', 'CAPTURED'))
            ) THEN
                RAISE EXCEPTION
                    'Invalid FLEX settlement transition: % -> %',
                    OLD.status, NEW.status
                    USING ERRCODE = '23514';
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_validate_flex_settlement_identity
BEFORE INSERT OR UPDATE ON flex_settlements
FOR EACH ROW
EXECUTE FUNCTION validate_flex_settlement_identity();

-- +goose Down

DROP TABLE IF EXISTS flex_settlements;
DROP FUNCTION IF EXISTS validate_flex_settlement_identity();
