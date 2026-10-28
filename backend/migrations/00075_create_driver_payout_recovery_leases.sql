-- +goose Up

-- Recovery coordination only.
-- This table does not authorize financial settlement,
-- provider transfer initiation or payout finalization.

CREATE TABLE driver_payout_recovery_leases (
    payout_id UUID PRIMARY KEY
        REFERENCES driver_payouts(id)
        ON DELETE CASCADE,

    lease_token UUID NOT NULL,

    lease_owner TEXT NOT NULL,

    lease_expires_at TIMESTAMPTZ NOT NULL,

    next_lookup_at TIMESTAMPTZ NOT NULL,

    last_lookup_at TIMESTAMPTZ,

    lookup_count BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL
        DEFAULT clock_timestamp(),

    updated_at TIMESTAMPTZ NOT NULL
        DEFAULT clock_timestamp(),

    CONSTRAINT chk_driver_payout_recovery_owner
        CHECK (length(btrim(lease_owner)) > 0),

    CONSTRAINT chk_driver_payout_recovery_lookup_count
        CHECK (lookup_count >= 0),

    CONSTRAINT chk_driver_payout_recovery_lookup_order
        CHECK (
            last_lookup_at IS NULL
            OR next_lookup_at >= last_lookup_at
        )
);

-- Supports finding expired leases and due lookups.
CREATE INDEX idx_driver_payout_recovery_due
ON driver_payout_recovery_leases (
    next_lookup_at,
    lease_expires_at
);

COMMENT ON TABLE driver_payout_recovery_leases IS
'Distributed coordination for lookup-only driver payout recovery; never authorizes transfer initiation or financial finalization';

COMMENT ON COLUMN driver_payout_recovery_leases.lease_token IS
'Unique fencing token; stale owners must not update or release a newer lease';

COMMENT ON COLUMN driver_payout_recovery_leases.next_lookup_at IS
'Durable cooldown deadline shared by all recovery workers';

-- +goose Down

DROP TABLE IF EXISTS driver_payout_recovery_leases;
