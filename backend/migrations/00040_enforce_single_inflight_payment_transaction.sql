-- +goose Up

-- ---------------------------------------------------------------------
-- CONNECT payment-operation concurrency invariant.
--
-- A logical payment may have at most one provider-facing operation
-- actively in flight at any time.
--
-- This prevents separate requests using different idempotency keys from
-- creating concurrent SALE/AUTHORIZE/CAPTURE/REFUND/VOID operations for
-- the same payment.
--
-- Terminal transactions do not participate in this constraint, allowing
-- later legitimate operations such as:
--
--   AUTHORIZE SUCCESS -> CAPTURE
--   PAID              -> REFUND
--   PARTIAL REFUND    -> another REFUND
-- ---------------------------------------------------------------------

CREATE UNIQUE INDEX uq_payment_transactions_single_inflight
ON payment_transactions (
    payment_id
)
WHERE status IN (
    'PENDING',
    'PROCESSING'
);


-- +goose Down

DROP INDEX IF EXISTS uq_payment_transactions_single_inflight;