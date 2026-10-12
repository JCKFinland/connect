-- +goose Up

-- +goose StatementBegin
DO $$
DECLARE
    has_old BOOLEAN;
    has_new BOOLEAN;
BEGIN
    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'vehicles'
          AND column_name = 'seat_capacity'
    ) INTO has_old;

    SELECT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'vehicles'
          AND column_name = 'seating_capacity'
    ) INTO has_new;

    IF has_old AND NOT has_new THEN
        ALTER TABLE public.vehicles
            RENAME COLUMN seat_capacity TO seating_capacity;
    ELSIF has_old = has_new THEN
        RAISE EXCEPTION
            'Unexpected vehicles capacity schema: seat_capacity=%, seating_capacity=%',
            has_old, has_new;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

-- No-op: migration 00009 already defines seating_capacity.
-- Reverting this compatibility migration must not rename that column
-- to the obsolete seat_capacity name.
