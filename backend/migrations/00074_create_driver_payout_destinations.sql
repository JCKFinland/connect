-- +goose Up

-- Provider destination identifiers are opaque provider references.
-- Do not store IBANs, bank credentials, or payment secrets here.

CREATE TABLE driver_payout_destinations
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    driver_id UUID NOT NULL
        REFERENCES users(id) ON DELETE RESTRICT,

    company_id UUID NOT NULL
        REFERENCES companies(id) ON DELETE RESTRICT,

    provider VARCHAR(50) NOT NULL,

    provider_destination_id VARCHAR(255) NOT NULL,

    verification_status VARCHAR(30) NOT NULL DEFAULT 'PENDING',

    verified_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_driver_payout_destination_provider
        CHECK (
            provider = UPPER(BTRIM(provider))
            AND provider <> ''
        ),

    CONSTRAINT chk_driver_payout_destination_identity
        CHECK (
            provider_destination_id = BTRIM(provider_destination_id)
            AND provider_destination_id <> ''
        ),

    CONSTRAINT chk_driver_payout_destination_verification
        CHECK (
            verification_status IN (
                'PENDING',
                'VERIFIED',
                'REVOKED'
            )
        ),

    CONSTRAINT chk_driver_payout_destination_verified_at
        CHECK (
            (
                verification_status = 'PENDING'
                AND verified_at IS NULL
            )
            OR (
                verification_status IN ('VERIFIED', 'REVOKED')
                AND verified_at IS NOT NULL
            )
        ),

    CONSTRAINT uq_driver_payout_destination_provider_identity
        UNIQUE (provider, provider_destination_id)
);

CREATE INDEX idx_driver_payout_destinations_owner
ON driver_payout_destinations(driver_id, company_id);

ALTER TABLE driver_payout_executions
ADD COLUMN destination_id UUID
REFERENCES driver_payout_destinations(id)
ON DELETE RESTRICT;

CREATE INDEX idx_driver_payout_execution_destination
ON driver_payout_executions(destination_id)
WHERE destination_id IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION protect_driver_payout_destination()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.verification_status <> 'PENDING'
           OR NEW.verified_at IS NOT NULL THEN
            RAISE EXCEPTION
                'new payout destination must start pending';
        END IF;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.driver_id IS DISTINCT FROM OLD.driver_id
           OR NEW.company_id IS DISTINCT FROM OLD.company_id
           OR NEW.provider IS DISTINCT FROM OLD.provider
           OR NEW.provider_destination_id
              IS DISTINCT FROM OLD.provider_destination_id
           THEN
            RAISE EXCEPTION
                'payout destination identity and verification are immutable';
        END IF;

        IF OLD.verification_status = 'PENDING' THEN
            IF NEW.verification_status = 'VERIFIED' THEN
                IF NEW.verified_at IS NULL THEN
                    RAISE EXCEPTION 'verification timestamp required';
                END IF;
            ELSIF NEW.verification_status <> 'PENDING'
                  OR NEW.verified_at IS NOT NULL THEN
                RAISE EXCEPTION 'invalid pending destination transition';
            END IF;
        ELSIF OLD.verification_status = 'VERIFIED' THEN
            IF NEW.verification_status NOT IN ('VERIFIED', 'REVOKED')
               OR NEW.verified_at IS DISTINCT FROM OLD.verified_at THEN
                RAISE EXCEPTION 'invalid verified destination transition';
            END IF;
        ELSE
            IF NEW.verification_status <> 'REVOKED'
               OR NEW.verified_at IS DISTINCT FROM OLD.verified_at THEN
                RAISE EXCEPTION 'revoked destination cannot change';
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_protect_driver_payout_destination
BEFORE INSERT OR UPDATE ON driver_payout_destinations
FOR EACH ROW
EXECUTE FUNCTION protect_driver_payout_destination();

-- The execution destination must match the payout owner and provider.
-- Historical unbound executions remain readable, but cannot start.

-- +goose StatementBegin
CREATE FUNCTION enforce_driver_payout_execution_destination()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    destination_record RECORD;
    payout_record RECORD;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF OLD.destination_id IS NOT NULL
           AND NEW.destination_id
               IS DISTINCT FROM OLD.destination_id THEN
            RAISE EXCEPTION
                'bound payout execution destination is immutable';
        END IF;

        IF OLD.request_started_at IS NOT NULL
           AND NEW.destination_id
               IS DISTINCT FROM OLD.destination_id THEN
            RAISE EXCEPTION
                'started payout execution destination cannot change';
        END IF;
    END IF;

    IF TG_OP = 'UPDATE'
       AND OLD.destination_id IS NULL
       AND NEW.destination_id IS NOT NULL
       AND (
           OLD.execution_status <> 'PENDING'
           OR OLD.attempt_count <> 0
           OR OLD.request_started_at IS NOT NULL
       ) THEN
        RAISE EXCEPTION
            'historical started execution cannot bind destination';
    END IF;

    IF NEW.destination_id IS NULL THEN
        IF TG_OP = 'INSERT' THEN
            RAISE EXCEPTION
                'new payout execution requires destination';
        END IF;

        IF OLD.destination_id IS NOT NULL THEN
            RAISE EXCEPTION
                'bound payout execution cannot lose destination';
        END IF;

        IF OLD.execution_status = 'PENDING'
           AND NEW.execution_status <> 'PENDING' THEN
            RAISE EXCEPTION
                'historical unbound payout execution cannot start';
        END IF;

        IF OLD.execution_status = 'PROCESSING'
           AND NEW.execution_status NOT IN (
               'PROCESSING',
               'REQUIRES_RECONCILIATION',
               'SUCCEEDED'
           ) THEN
            RAISE EXCEPTION
                'invalid historical processing recovery';
        END IF;

        IF OLD.execution_status = 'REQUIRES_RECONCILIATION'
           AND NEW.execution_status NOT IN (
               'REQUIRES_RECONCILIATION',
               'SUCCEEDED'
           ) THEN
            RAISE EXCEPTION
                'invalid historical reconciliation recovery';
        END IF;

        RETURN NEW;
    END IF;

    SELECT
        driver_id,
        company_id,
        provider,
        verification_status
    INTO destination_record
    FROM driver_payout_destinations
    WHERE id = NEW.destination_id
    FOR SHARE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'payout destination not found';
    END IF;

    SELECT driver_id, company_id
    INTO payout_record
    FROM driver_payouts
    WHERE id = NEW.payout_id;

    IF NOT FOUND
       OR destination_record.driver_id <> payout_record.driver_id
       OR destination_record.company_id <> payout_record.company_id
       OR destination_record.provider <> NEW.provider THEN
        RAISE EXCEPTION
            'payout destination ownership or provider mismatch';
    END IF;

    IF (
        TG_OP = 'INSERT'
        OR (
            TG_OP = 'UPDATE'
            AND NEW.destination_id IS DISTINCT FROM OLD.destination_id
        )
        OR NEW.execution_status = 'PROCESSING'
    )
    AND destination_record.verification_status <> 'VERIFIED' THEN
        RAISE EXCEPTION
            'payout destination is not verified';
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_enforce_driver_payout_execution_destination
BEFORE INSERT OR UPDATE ON driver_payout_executions
FOR EACH ROW
EXECUTE FUNCTION enforce_driver_payout_execution_destination();

-- +goose Down

DROP TRIGGER IF EXISTS
trg_enforce_driver_payout_execution_destination
ON driver_payout_executions;

DROP FUNCTION IF EXISTS
enforce_driver_payout_execution_destination();

DROP TRIGGER IF EXISTS
trg_protect_driver_payout_destination
ON driver_payout_destinations;

DROP FUNCTION IF EXISTS
protect_driver_payout_destination();

DROP INDEX IF EXISTS idx_driver_payout_execution_destination;

ALTER TABLE driver_payout_executions
DROP COLUMN IF EXISTS destination_id;

DROP TABLE IF EXISTS driver_payout_destinations;
