-- +goose Up

CREATE TABLE IF NOT EXISTS driver_payout_items
(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    payout_id UUID NOT NULL,

    earning_id UUID NOT NULL UNIQUE,

    amount NUMERIC(10,2) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_driver_payout_items_payout
        FOREIGN KEY (payout_id)
        REFERENCES driver_payouts(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_driver_payout_items_earning
        FOREIGN KEY (earning_id)
        REFERENCES driver_earnings(id)
        ON DELETE RESTRICT,

    CONSTRAINT chk_driver_payout_item_amount
        CHECK (amount >= 0)
);

CREATE INDEX IF NOT EXISTS idx_driver_payout_items_payout
ON driver_payout_items(payout_id);

CREATE INDEX IF NOT EXISTS idx_driver_payout_items_earning
ON driver_payout_items(earning_id);

-- +goose Down

DROP TABLE IF EXISTS driver_payout_items;