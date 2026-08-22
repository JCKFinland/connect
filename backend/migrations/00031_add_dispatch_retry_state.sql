-- +goose Up

ALTER TABLE ride_requests
ADD COLUMN IF NOT EXISTS dispatch_retry_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE ride_requests
ADD COLUMN IF NOT EXISTS next_dispatch_attempt_at TIMESTAMPTZ;

ALTER TABLE ride_requests
ADD COLUMN IF NOT EXISTS last_dispatch_attempt_at TIMESTAMPTZ;

ALTER TABLE ride_requests
ADD CONSTRAINT chk_ride_requests_dispatch_retry_count
CHECK (dispatch_retry_count >= 0);

CREATE INDEX IF NOT EXISTS idx_ride_requests_dispatch_retry
ON ride_requests (
    status,
    next_dispatch_attempt_at,
    requested_at
)
WHERE status = 'PENDING';


-- +goose Down

DROP INDEX IF EXISTS idx_ride_requests_dispatch_retry;

ALTER TABLE ride_requests
DROP CONSTRAINT IF EXISTS chk_ride_requests_dispatch_retry_count;

ALTER TABLE ride_requests
DROP COLUMN IF EXISTS last_dispatch_attempt_at;

ALTER TABLE ride_requests
DROP COLUMN IF EXISTS next_dispatch_attempt_at;

ALTER TABLE ride_requests
DROP COLUMN IF EXISTS dispatch_retry_count;