-- +goose Up

CREATE TABLE driver_payout_executions
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    payout_id UUID NOT NULL UNIQUE
        REFERENCES driver_payouts(id)
        ON DELETE RESTRICT,

    provider VARCHAR(50) NOT NULL,

    idempotency_key VARCHAR(255) NOT NULL UNIQUE,

    amount NUMERIC(10,2) NOT NULL,

    currency CHAR(3) NOT NULL,

    execution_status VARCHAR(40) NOT NULL DEFAULT 'PENDING',

    provider_reference VARCHAR(255),

    request_started_at TIMESTAMPTZ,

    provider_confirmed_at TIMESTAMPTZ,

    attempt_count INTEGER NOT NULL DEFAULT 0,

    last_error TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_driver_payout_execution_provider
        CHECK (BTRIM(provider) <> ''),

    CONSTRAINT chk_driver_payout_execution_key
        CHECK (BTRIM(idempotency_key) <> ''),

    CONSTRAINT chk_driver_payout_execution_amount
        CHECK (amount > 0),

    CONSTRAINT chk_driver_payout_execution_currency
        CHECK (currency = 'EUR'),

    CONSTRAINT chk_driver_payout_execution_status
        CHECK (
            execution_status IN (
                'PENDING',
                'PROCESSING',
                'SUCCEEDED',
                'FAILED',
                'REQUIRES_RECONCILIATION'
            )
        ),

    CONSTRAINT chk_driver_payout_execution_attempts
        CHECK (attempt_count >= 0),

    CONSTRAINT chk_driver_payout_execution_request_state
        CHECK (
            (
                execution_status = 'PENDING'
                AND request_started_at IS NULL
                AND attempt_count = 0
            )
            OR (
                execution_status <> 'PENDING'
                AND request_started_at IS NOT NULL
                AND attempt_count > 0
            )
        ),

    CONSTRAINT chk_driver_payout_execution_success
        CHECK (
            execution_status <> 'SUCCEEDED'
            OR (
                provider_reference IS NOT NULL
                AND BTRIM(provider_reference) <> ''
                AND provider_confirmed_at IS NOT NULL
                AND request_started_at IS NOT NULL
                AND attempt_count > 0
            )
        ),

    CONSTRAINT chk_driver_payout_execution_confirmation
        CHECK (
            provider_confirmed_at IS NULL
            OR (
                provider_reference IS NOT NULL
                AND BTRIM(provider_reference) <> ''
            )
        )
);

CREATE UNIQUE INDEX uq_driver_payout_execution_provider_reference
ON driver_payout_executions(provider, provider_reference)
WHERE provider_reference IS NOT NULL;

CREATE INDEX idx_driver_payout_execution_status
ON driver_payout_executions(execution_status);

CREATE INDEX idx_driver_payout_execution_request_started
ON driver_payout_executions(request_started_at)
WHERE execution_status IN (
    'PROCESSING',
    'REQUIRES_RECONCILIATION'
);


-- Execution creation and updates must preserve financial identity.
-- Lock the parent payout while validating execution creation.

-- +goose StatementBegin
CREATE FUNCTION protect_driver_payout_execution_integrity()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    parent_payout RECORD;
BEGIN
    IF TG_OP = 'UPDATE' THEN

        IF NEW.payout_id IS DISTINCT FROM OLD.payout_id
           OR NEW.provider IS DISTINCT FROM OLD.provider
           OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
           OR NEW.amount IS DISTINCT FROM OLD.amount
           OR NEW.currency IS DISTINCT FROM OLD.currency THEN
            RAISE EXCEPTION
                'driver payout execution financial identity is immutable';
        END IF;

        IF OLD.provider_reference IS NOT NULL
           AND NEW.provider_reference
               IS DISTINCT FROM OLD.provider_reference THEN
            RAISE EXCEPTION
                'driver payout provider reference is immutable';
        END IF;

        IF OLD.provider_confirmed_at IS NOT NULL
           AND NEW.provider_confirmed_at
               IS DISTINCT FROM OLD.provider_confirmed_at THEN
            RAISE EXCEPTION
                'driver payout provider confirmation is immutable';
        END IF;

        IF OLD.request_started_at IS NOT NULL
           AND NEW.request_started_at
               IS DISTINCT FROM OLD.request_started_at THEN
            RAISE EXCEPTION
                'driver payout request timestamp is immutable';
        END IF;

        IF NEW.attempt_count < OLD.attempt_count THEN
            RAISE EXCEPTION
                'driver payout execution attempt count cannot decrease';
        END IF;

        IF OLD.execution_status IN ('SUCCEEDED', 'FAILED')
           AND NEW.execution_status
               IS DISTINCT FROM OLD.execution_status THEN
            RAISE EXCEPTION
                'terminal driver payout execution cannot reopen';
        END IF;

        IF OLD.execution_status = 'PENDING'
           AND NEW.execution_status NOT IN (
               'PENDING',
               'PROCESSING'
           ) THEN
            RAISE EXCEPTION
                'invalid pending payout execution transition';
        END IF;

        IF OLD.execution_status = 'PROCESSING'
           AND NEW.execution_status NOT IN (
               'PROCESSING',
               'SUCCEEDED',
               'FAILED',
               'REQUIRES_RECONCILIATION'
           ) THEN
            RAISE EXCEPTION
                'invalid processing payout execution transition';
        END IF;

        IF OLD.execution_status = 'REQUIRES_RECONCILIATION'
           AND NEW.execution_status NOT IN (
               'REQUIRES_RECONCILIATION',
               'SUCCEEDED',
               'FAILED'
           ) THEN
            RAISE EXCEPTION
                'invalid payout reconciliation transition';
        END IF;

        RETURN NEW;
    END IF;

    SELECT
        id,
        net_amount,
        currency,
        payout_status,
        payout_method
    INTO parent_payout
    FROM driver_payouts
    WHERE id = NEW.payout_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION
            'driver payout execution parent not found';
    END IF;

    IF parent_payout.payout_status <> 'PENDING'
       OR parent_payout.payout_method <> 'BANK_TRANSFER'
       OR parent_payout.net_amount <> NEW.amount
       OR parent_payout.currency <> NEW.currency THEN
        RAISE EXCEPTION
            'driver payout execution does not match prepared payout';
    END IF;

    IF NEW.execution_status <> 'PENDING'
       OR NEW.request_started_at IS NOT NULL
       OR NEW.provider_reference IS NOT NULL
       OR NEW.provider_confirmed_at IS NOT NULL
       OR NEW.attempt_count <> 0 THEN
        RAISE EXCEPTION
            'new payout execution must start unsubmitted';
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_protect_driver_payout_execution_integrity
BEFORE INSERT OR UPDATE
ON driver_payout_executions
FOR EACH ROW
EXECUTE FUNCTION protect_driver_payout_execution_integrity();


-- Locking contract:
-- Execution INSERT locks its parent payout FOR UPDATE.
-- Parent UPDATE already holds that row lock, so these operations
-- serialize before checking whether an execution exists.

-- +goose StatementBegin
CREATE FUNCTION protect_driver_payout_execution_parent()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM driver_payout_executions
        WHERE payout_id = OLD.id
    ) THEN
        IF NEW.driver_id IS DISTINCT FROM OLD.driver_id
           OR NEW.company_id IS DISTINCT FROM OLD.company_id
           OR NEW.payout_reference IS DISTINCT FROM OLD.payout_reference
           OR NEW.net_amount IS DISTINCT FROM OLD.net_amount
           OR NEW.gross_amount IS DISTINCT FROM OLD.gross_amount
           OR NEW.deductions IS DISTINCT FROM OLD.deductions
           OR NEW.adjustments IS DISTINCT FROM OLD.adjustments
           OR NEW.currency IS DISTINCT FROM OLD.currency
           OR NEW.payout_method IS DISTINCT FROM OLD.payout_method
           OR NEW.period_start IS DISTINCT FROM OLD.period_start
           OR NEW.period_end IS DISTINCT FROM OLD.period_end THEN
            RAISE EXCEPTION
                'executing driver payout financial identity is immutable';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_protect_driver_payout_execution_parent
BEFORE UPDATE ON driver_payouts
FOR EACH ROW
EXECUTE FUNCTION protect_driver_payout_execution_parent();

-- +goose Down

DROP TRIGGER IF EXISTS trg_protect_driver_payout_execution_integrity
ON driver_payout_executions;

DROP FUNCTION IF EXISTS protect_driver_payout_execution_integrity();

DROP TRIGGER IF EXISTS trg_protect_driver_payout_execution_parent
ON driver_payouts;

DROP FUNCTION IF EXISTS protect_driver_payout_execution_parent();

DROP TABLE IF EXISTS driver_payout_executions;
