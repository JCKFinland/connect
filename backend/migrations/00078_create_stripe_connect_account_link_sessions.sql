-- +goose Up

-- CONNECT: server-established Stripe Connect account-linking sessions.
--
-- A session is an intent, not Stripe ownership evidence.
-- No verified ownership writer or payout activation is introduced.
--
-- The consuming application must independently establish actor
-- authentication, permission, company membership, driver-company
-- relationship, and trusted Stripe account control.
--
-- The conditional UPDATE below is the intended atomic consumption
-- primitive. Validation alone does not consume a session.

CREATE TABLE stripe_connect_account_link_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    actor_user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE RESTRICT,

    driver_user_id UUID NOT NULL
        REFERENCES users(id) ON DELETE RESTRICT,

    company_id UUID NOT NULL
        REFERENCES companies(id) ON DELETE RESTRICT,

    stripe_account_id VARCHAR(255) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL
        DEFAULT clock_timestamp(),

    expires_at TIMESTAMPTZ NOT NULL,

    consumed_at TIMESTAMPTZ,

    CONSTRAINT chk_stripe_link_account_identity
        CHECK (
            stripe_account_id ~ '^acct_[A-Za-z0-9]{8,64}$'
        ),

    CONSTRAINT chk_stripe_link_lifetime
        CHECK (
            expires_at > created_at
            AND expires_at <= created_at + INTERVAL '15 minutes'
        ),

    CONSTRAINT chk_stripe_link_consumption
        CHECK (
            consumed_at IS NULL
            OR consumed_at >= created_at
        )
);

CREATE INDEX idx_stripe_link_actor_pending
ON stripe_connect_account_link_sessions (
    actor_user_id,
    expires_at
)
WHERE consumed_at IS NULL;

CREATE INDEX idx_stripe_link_driver_company
ON stripe_connect_account_link_sessions (
    driver_user_id,
    company_id
);

-- +goose StatementBegin
CREATE FUNCTION protect_stripe_connect_account_link_session()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.consumed_at IS NOT NULL
           OR NEW.created_at > clock_timestamp()
           OR NEW.expires_at <= clock_timestamp() THEN
            RAISE EXCEPTION
                'Stripe account-link sessions must start unconsumed and active';
        END IF;

        RETURN NEW;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.id IS DISTINCT FROM OLD.id
           OR NEW.actor_user_id IS DISTINCT FROM OLD.actor_user_id
           OR NEW.driver_user_id IS DISTINCT FROM OLD.driver_user_id
           OR NEW.company_id IS DISTINCT FROM OLD.company_id
           OR NEW.stripe_account_id IS DISTINCT FROM OLD.stripe_account_id
           OR NEW.created_at IS DISTINCT FROM OLD.created_at
           OR NEW.expires_at IS DISTINCT FROM OLD.expires_at THEN
            RAISE EXCEPTION
                'Stripe account-link session identity is immutable';
        END IF;

        IF OLD.consumed_at IS NOT NULL
           AND NEW IS DISTINCT FROM OLD THEN
            RAISE EXCEPTION
                'Consumed Stripe account-link sessions are immutable';
        END IF;

        IF OLD.consumed_at IS NULL
           AND NEW.consumed_at IS NOT NULL THEN
            IF clock_timestamp() >= OLD.expires_at
               OR NEW.consumed_at < OLD.created_at
               OR NEW.consumed_at > clock_timestamp() THEN
                RAISE EXCEPTION
                    'Invalid Stripe account-link session consumption';
            END IF;
        END IF;

        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            'Stripe account-link session deletion is disabled';
    END IF;

    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_protect_stripe_connect_account_link_session
BEFORE INSERT OR UPDATE OR DELETE
ON stripe_connect_account_link_sessions
FOR EACH ROW
EXECUTE FUNCTION protect_stripe_connect_account_link_session();

COMMENT ON TABLE stripe_connect_account_link_sessions IS
'Single-use linking intents; never independent Stripe ownership evidence';

-- Atomic consumption must use this conditional pattern:
--
-- UPDATE stripe_connect_account_link_sessions
-- SET consumed_at = clock_timestamp()
-- WHERE id = $1
--   AND actor_user_id = $2
--   AND driver_user_id = $3
--   AND company_id = $4
--   AND stripe_account_id = $5
--   AND consumed_at IS NULL
--   AND created_at <= clock_timestamp()
--   AND expires_at > clock_timestamp()
-- RETURNING id;
--
-- Exactly one returned row means the claim succeeded.
-- Zero rows means missing, expired, consumed, or mismatched.
-- This claim is not proof of Stripe ownership.

-- +goose Down

DROP TABLE IF EXISTS stripe_connect_account_link_sessions;

DROP FUNCTION IF EXISTS
    protect_stripe_connect_account_link_session();
