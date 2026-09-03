-- +goose Up

-- ---------------------------------------------------------------------
-- CONNECT payment integrity hardening.
--
-- A payment must derive its monetary identity from the authoritative
-- fare produced during trip completion.
--
-- The composite foreign key below guarantees that:
--
--   payments.fare_id   belongs to payments.trip_id
--   payments.amount    equals trip_fares.total_amount
--   payments.currency  equals trip_fares.currency
--
-- PostgreSQL requires the referenced column set to have a UNIQUE
-- constraint, even though trip_fares.id is already individually unique.
-- ---------------------------------------------------------------------

ALTER TABLE trip_fares
ADD CONSTRAINT uq_trip_fares_payment_identity
UNIQUE (
    id,
    trip_id,
    total_amount,
    currency
);

ALTER TABLE payments
ADD CONSTRAINT fk_payments_authoritative_fare
FOREIGN KEY (
    fare_id,
    trip_id,
    amount,
    currency
)
REFERENCES trip_fares (
    id,
    trip_id,
    total_amount,
    currency
)
ON DELETE RESTRICT;

-- ---------------------------------------------------------------------
-- A provider idempotency key may be NULL for legacy/non-provider
-- records, but when supplied it must not be reusable for the same
-- payment provider.
--
-- The provider is intentionally part of the key because separate
-- providers may legitimately use the same external idempotency value.
-- ---------------------------------------------------------------------

CREATE UNIQUE INDEX uq_payment_transactions_provider_idempotency
ON payment_transactions (
    provider,
    idempotency_key
)
WHERE idempotency_key IS NOT NULL;


-- +goose Down

DROP INDEX IF EXISTS uq_payment_transactions_provider_idempotency;

ALTER TABLE payments
DROP CONSTRAINT IF EXISTS fk_payments_authoritative_fare;

ALTER TABLE trip_fares
DROP CONSTRAINT IF EXISTS uq_trip_fares_payment_identity;