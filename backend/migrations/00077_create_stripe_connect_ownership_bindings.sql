-- +goose Up

-- CONNECT: Stripe Connect ownership-binding evidence.
--
-- A binding is NOT proof of Stripe account ownership.
-- Only a future independently authorized verification
-- workflow may establish verified evidence.
--
-- This migration creates no API, role grant, payout
-- destination verification, or financial execution.

CREATE TABLE stripe_connect_ownership_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    actor_user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE RESTRICT,

    driver_user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE RESTRICT,

    company_id UUID NOT NULL
        REFERENCES companies(id) ON DELETE RESTRICT,

    stripe_account_id VARCHAR(255) NOT NULL,

    verification_status VARCHAR(20) NOT NULL
        DEFAULT 'PENDING',

    evidence_source VARCHAR(80),

    evidence_reference VARCHAR(255),

    verified_by_user_id UUID
        REFERENCES users(id) ON DELETE RESTRICT,

    verified_at TIMESTAMPTZ,

    revoked_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL
        DEFAULT clock_timestamp(),

    CONSTRAINT chk_stripe_ownership_account
        CHECK (
            stripe_account_id ~ '^acct_[A-Za-z0-9]{8,64}$'
        ),

    CONSTRAINT chk_stripe_ownership_status
        CHECK (
            verification_status IN (
                'PENDING',
                'VERIFIED',
                'REVOKED'
            )
        ),

    CONSTRAINT chk_stripe_ownership_evidence
        CHECK (
            (
                verification_status = 'PENDING'
                AND evidence_source IS NULL
                AND evidence_reference IS NULL
                AND verified_by_user_id IS NULL
                AND verified_at IS NULL
                AND revoked_at IS NULL
            )
            OR (
                verification_status = 'VERIFIED'
                AND evidence_source IS NOT NULL
                AND length(btrim(evidence_source)) > 0
                AND evidence_reference IS NOT NULL
                AND length(btrim(evidence_reference)) > 0
                AND verified_by_user_id IS NOT NULL
                AND verified_at IS NOT NULL
                AND revoked_at IS NULL
            )
            OR (
                verification_status = 'REVOKED'
                AND evidence_source IS NOT NULL
                AND length(btrim(evidence_source)) > 0
                AND evidence_reference IS NOT NULL
                AND length(btrim(evidence_reference)) > 0
                AND verified_by_user_id IS NOT NULL
                AND verified_at IS NOT NULL
                AND revoked_at IS NOT NULL
                AND revoked_at >= verified_at
            )
        )
);

CREATE UNIQUE INDEX uq_stripe_ownership_active_account
ON stripe_connect_ownership_bindings(stripe_account_id)
WHERE verification_status IN ('PENDING', 'VERIFIED');

CREATE INDEX idx_stripe_ownership_driver_company
ON stripe_connect_ownership_bindings(
    driver_user_id,
    company_id,
    verification_status
);

-- +goose StatementBegin
CREATE FUNCTION protect_stripe_connect_ownership_binding()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.verification_status <> 'PENDING'
           OR NEW.evidence_source IS NOT NULL
           OR NEW.evidence_reference IS NOT NULL
           OR NEW.verified_by_user_id IS NOT NULL
           OR NEW.verified_at IS NOT NULL
           OR NEW.revoked_at IS NOT NULL THEN
            RAISE EXCEPTION
                'Stripe ownership bindings must start pending';
        END IF;

        RETURN NEW;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.actor_user_id IS DISTINCT FROM OLD.actor_user_id
           OR NEW.driver_user_id IS DISTINCT FROM OLD.driver_user_id
           OR NEW.company_id IS DISTINCT FROM OLD.company_id
           OR NEW.stripe_account_id IS DISTINCT FROM OLD.stripe_account_id
           OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
            RAISE EXCEPTION
                'Stripe ownership binding identity is immutable';
        END IF;

        -- Verification is deliberately unavailable.
        -- A later reviewed migration must introduce
        -- a trusted transition path with audit evidence.
        IF NEW IS DISTINCT FROM OLD THEN
            RAISE EXCEPTION
                'Stripe ownership binding updates are disabled';
        END IF;

        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            'Stripe ownership binding deletion is disabled';
    END IF;

    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_protect_stripe_connect_ownership_binding
BEFORE INSERT OR UPDATE OR DELETE
ON stripe_connect_ownership_bindings
FOR EACH ROW
EXECUTE FUNCTION protect_stripe_connect_ownership_binding();

COMMENT ON TABLE stripe_connect_ownership_bindings IS
'Pending-only Stripe ownership records; no production verification writer is authorized';

-- +goose Down

DROP TABLE IF EXISTS stripe_connect_ownership_bindings;

DROP FUNCTION IF EXISTS
    protect_stripe_connect_ownership_binding();
