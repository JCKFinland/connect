-- +goose Up

DROP INDEX IF EXISTS uq_payment_transactions_provider_transaction;

CREATE INDEX IF NOT EXISTS idx_payment_transactions_provider_transaction
ON payment_transactions(provider, provider_transaction_id)
WHERE provider_transaction_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_payment_transactions_provider_transaction;

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_transactions_provider_transaction
ON payment_transactions(provider, provider_transaction_id)
WHERE provider_transaction_id IS NOT NULL;