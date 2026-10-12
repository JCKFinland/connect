-- +goose Up

-- CONNECT supports mutually exclusive matching workflows.
--
-- STANDARD retains the existing dispatch lifecycle.
-- FLEX is reserved for negotiated-fare auction processing.
--
-- Existing requests remain STANDARD.
-- New requests default to STANDARD until explicitly activated.

ALTER TABLE ride_requests
    ADD COLUMN matching_mode VARCHAR(20)
    NOT NULL DEFAULT 'STANDARD';

ALTER TABLE ride_requests
    ADD CONSTRAINT chk_ride_requests_matching_mode
    CHECK (matching_mode IN ('STANDARD', 'FLEX'));

CREATE INDEX idx_ride_requests_matching_mode_status
    ON ride_requests(matching_mode, status);

-- +goose Down

DROP INDEX IF EXISTS idx_ride_requests_matching_mode_status;

ALTER TABLE ride_requests
    DROP CONSTRAINT IF EXISTS chk_ride_requests_matching_mode;

ALTER TABLE ride_requests
    DROP COLUMN IF EXISTS matching_mode;
