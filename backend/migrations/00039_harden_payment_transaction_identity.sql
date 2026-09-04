-- +goose Up

-- ---------------------------------------------------------------------
-- CONNECT payment-provider transaction identity hardening.
--
-- External payment providers may retry callbacks or delivery events.
-- When a provider transaction identifier is supplied, the same provider
-- transaction must never be represented by more than one CONNECT
-- payment_transactions row.
--
-- provider_transaction_id remains nullable because a local transaction
-- may legitimately exist before a provider has assigned its identifier.
-- ---------------------------------------------------------------------

CREATE UNIQUE INDEX uq_payment_transactions_provider_transaction
ON payment_transactions (
    provider,
    provider_transaction_id
)
WHERE provider_transaction_id IS NOT NULL;


-- +goose Down

DROP INDEX IF EXISTS uq_payment_transactions_provider_transaction;