-- +goose Up
-- Preserve historical rows without inventing an assignment identity.
-- Acceptance rejects legacy NULL pins, including pending legacy offers.
ALTER TABLE dispatch_offers
    ADD COLUMN assignment_id UUID;

ALTER TABLE dispatch_offers
    ADD CONSTRAINT fk_dispatch_offers_assignment
    FOREIGN KEY (assignment_id)
    REFERENCES driver_assignments(id)
    ON DELETE RESTRICT;

CREATE INDEX idx_dispatch_offers_assignment_id
    ON dispatch_offers(assignment_id)
    WHERE assignment_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_dispatch_offers_assignment_id;
ALTER TABLE dispatch_offers
    DROP CONSTRAINT IF EXISTS fk_dispatch_offers_assignment;
ALTER TABLE dispatch_offers
    DROP COLUMN IF EXISTS assignment_id;
