-- +goose Up

-- CONNECT: Durable Stripe Connect account-provisioning intent.
--
-- A provisioning intent is NOT evidence of Stripe account ownership,
-- completed onboarding, destination verification, or payout authority.
--
-- The idempotency key must be reused for the same Stripe operation.
-- Ambiguous provider outcomes must not generate a fresh key.
--
-- No provider call, HTTP route, or financial execution is enabled.

CREATE TABLE stripe_connect_provisioning_intents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    actor_user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE RESTRICT,

    driver_user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE RESTRICT,

    company_id UUID NOT NULL
        REFERENCES companies(id) ON DELETE RESTRICT,

    idempotency_key VARCHAR(255) NOT NULL,

    stripe_account_id VARCHAR(255),

    provisioning_status VARCHAR(30) NOT NULL
        DEFAULT 'PENDING',

    request_started_at TIMESTAMPTZ,

    account_recorded_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL
        DEFAULT transaction_timestamp(),

    updated_at TIMESTAMPTZ NOT NULL
        DEFAULT transaction_timestamp(),

    CONSTRAINT uq_stripe_provisioning_driver_company
        UNIQUE (driver_user_id, company_id),

    CONSTRAINT uq_stripe_provisioning_idempotency
        UNIQUE (idempotency_key),

    CONSTRAINT uq_stripe_provisioning_account
        UNIQUE (stripe_account_id),

    CONSTRAINT chk_stripe_provisioning_key
        CHECK (
            length(btrim(idempotency_key)) > 0
            AND idempotency_key = btrim(idempotency_key)
        ),

    CONSTRAINT chk_stripe_provisioning_account
        CHECK (
            stripe_account_id IS NULL
            OR stripe_account_id ~ '^acct_[A-Za-z0-9]{8,64}$'
        ),

    CONSTRAINT chk_stripe_provisioning_status
        CHECK (
            provisioning_status IN (
                'PENDING',
                'REQUESTED',
                'REQUIRES_RECONCILIATION',
                'ACCOUNT_RECORDED'
            )
        ),

    CONSTRAINT chk_stripe_provisioning_lifecycle
        CHECK (
            (
                provisioning_status = 'PENDING'
                AND request_started_at IS NULL
                AND stripe_account_id IS NULL
                AND account_recorded_at IS NULL
            )
            OR (
                provisioning_status IN (
                    'REQUESTED',
                    'REQUIRES_RECONCILIATION'
                )
                AND request_started_at IS NOT NULL
                AND stripe_account_id IS NULL
                AND account_recorded_at IS NULL
            )
            OR (
                provisioning_status = 'ACCOUNT_RECORDED'
                AND request_started_at IS NOT NULL
                AND stripe_account_id IS NOT NULL
                AND account_recorded_at IS NOT NULL
                AND account_recorded_at >= request_started_at
            )
        ),

    CONSTRAINT chk_stripe_provisioning_timestamps
        CHECK (
            updated_at >= created_at
            AND (
                request_started_at IS NULL
                OR request_started_at >= created_at
            )
        )
);

CREATE INDEX idx_stripe_provisioning_status
ON stripe_connect_provisioning_intents (
    provisioning_status,
    created_at
);

-- +goose StatementBegin
CREATE FUNCTION protect_stripe_connect_provisioning_intent()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.provisioning_status <> 'PENDING'
           OR NEW.request_started_at IS NOT NULL
           OR NEW.stripe_account_id IS NOT NULL
           OR NEW.account_recorded_at IS NOT NULL
           OR NEW.created_at > clock_timestamp()
           OR NEW.updated_at <> NEW.created_at THEN
            RAISE EXCEPTION
                'Stripe provisioning intents must start pending';
        END IF;

        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            'Stripe provisioning intent deletion is disabled';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.id IS DISTINCT FROM OLD.id
           OR NEW.actor_user_id IS DISTINCT FROM OLD.actor_user_id
           OR NEW.driver_user_id IS DISTINCT FROM OLD.driver_user_id
           OR NEW.company_id IS DISTINCT FROM OLD.company_id
           OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
           OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
            RAISE EXCEPTION
                'Stripe provisioning identity is immutable';
        END IF;

        IF OLD.provisioning_status = 'PENDING' THEN
            IF NEW.provisioning_status <> 'REQUESTED'
               OR NEW.request_started_at IS NULL
               OR NEW.stripe_account_id IS NOT NULL
               OR NEW.account_recorded_at IS NOT NULL THEN
                RAISE EXCEPTION
                    'Invalid Stripe provisioning start';
            END IF;

            NEW.request_started_at := clock_timestamp();

        ELSIF OLD.provisioning_status IN (
            'REQUESTED',
            'REQUIRES_RECONCILIATION'
        ) THEN
            IF NEW.request_started_at IS DISTINCT FROM
               OLD.request_started_at THEN
                RAISE EXCEPTION
                    'Stripe provisioning request identity is immutable';
            END IF;

            IF NEW.provisioning_status = 'ACCOUNT_RECORDED' THEN
                IF NEW.stripe_account_id IS NULL
                   OR NEW.account_recorded_at IS NULL THEN
                    RAISE EXCEPTION
                        'Stripe provisioning account evidence missing';
                END IF;

                NEW.account_recorded_at := clock_timestamp();

            ELSIF NEW.provisioning_status =
                  'REQUIRES_RECONCILIATION' THEN
                IF NEW.stripe_account_id IS NOT NULL
                   OR NEW.account_recorded_at IS NOT NULL THEN
                    RAISE EXCEPTION
                        'Unresolved provisioning cannot record an account';
                END IF;

            ELSE
                RAISE EXCEPTION
                    'Invalid Stripe provisioning transition';
            END IF;

        ELSE
            RAISE EXCEPTION
                'Recorded Stripe provisioning is immutable';
        END IF;

        NEW.updated_at := clock_timestamp();
        RETURN NEW;
    END IF;

    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_protect_stripe_connect_provisioning_intent
BEFORE INSERT OR UPDATE OR DELETE
ON stripe_connect_provisioning_intents
FOR EACH ROW
EXECUTE FUNCTION protect_stripe_connect_provisioning_intent();

COMMENT ON TABLE stripe_connect_provisioning_intents IS
'Durable Stripe provisioning operations; no ownership verification or payout authority';

-- +goose Down

DROP TABLE IF EXISTS stripe_connect_provisioning_intents;

DROP FUNCTION IF EXISTS
    protect_stripe_connect_provisioning_intent();
