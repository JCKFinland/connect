-- +goose Up

-- CONNECT: privileged payout destination verification.
--
-- This migration creates a permission only.
-- It does not grant the permission to any role,
-- verify any destination, or enable payouts.

INSERT INTO permissions (name, description)
VALUES (
    'payout.destinations.verify',
    'Verify payout destinations after independent ownership and eligibility checks'
)
ON CONFLICT (name) DO NOTHING;

-- +goose Down

DELETE FROM permissions
WHERE name = 'payout.destinations.verify';
