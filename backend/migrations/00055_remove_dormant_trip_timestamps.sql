-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM trips
        WHERE passenger_on_board_at IS NOT NULL
           OR pickup_at IS NOT NULL
    ) THEN
        RAISE EXCEPTION
            'cannot remove dormant trip timestamps: passenger_on_board_at or pickup_at contains data';
    END IF;
END
$$;

ALTER TABLE trips
    DROP COLUMN passenger_on_board_at,
    DROP COLUMN pickup_at;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE trips
    ADD COLUMN passenger_on_board_at TIMESTAMPTZ,
    ADD COLUMN pickup_at TIMESTAMPTZ;
-- +goose StatementEnd
