-- +goose Up

ALTER TABLE ride_requests
ADD COLUMN standard_dispatch_started_at TIMESTAMPTZ;

-- Historical offers and trips establish that standard dispatch
-- has already begun, even if retry counters were reset.
UPDATE ride_requests AS rr
SET standard_dispatch_started_at = COALESCE(
    (
        SELECT MIN(d.offered_at)
        FROM dispatch_offers AS d
        WHERE d.ride_request_id = rr.id
    ),
    (
        SELECT MIN(t.created_at)
        FROM trips AS t
        WHERE t.ride_request_id = rr.id
    ),
    rr.last_dispatch_attempt_at
)
WHERE rr.matching_mode = 'STANDARD'
  AND (
      EXISTS (
          SELECT 1
          FROM dispatch_offers AS d
          WHERE d.ride_request_id = rr.id
      )
      OR EXISTS (
          SELECT 1
          FROM trips AS t
          WHERE t.ride_request_id = rr.id
      )
      OR rr.last_dispatch_attempt_at IS NOT NULL
  );

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION protect_standard_dispatch_started_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND OLD.standard_dispatch_started_at IS NOT NULL
       AND NEW.standard_dispatch_started_at IS DISTINCT FROM
           OLD.standard_dispatch_started_at THEN
        RAISE EXCEPTION
            'standard_dispatch_started_at is immutable once set';
    END IF;

    IF NEW.standard_dispatch_started_at IS NOT NULL
       AND NEW.matching_mode <> 'STANDARD' THEN
        RAISE EXCEPTION
            'standard dispatch history requires STANDARD matching mode';
    END IF;

    RETURN NEW;
END;
$$;

-- +goose StatementEnd

CREATE TRIGGER trg_protect_standard_dispatch_started_at
BEFORE INSERT OR UPDATE ON ride_requests
FOR EACH ROW
EXECUTE FUNCTION protect_standard_dispatch_started_at();

-- +goose Down

DROP TRIGGER IF EXISTS trg_protect_standard_dispatch_started_at
ON ride_requests;

DROP FUNCTION IF EXISTS protect_standard_dispatch_started_at();

ALTER TABLE ride_requests
DROP COLUMN IF EXISTS standard_dispatch_started_at;
