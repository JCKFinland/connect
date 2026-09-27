package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *DriverEarningRepository) GetByTripID(
	ctx context.Context,
	tripID string,
) (*models.DriverEarning, error) {
	const query = `
		SELECT
			` + driverEarningColumns + `
		FROM driver_earnings
		WHERE trip_id = $1
	`

	earning, err := scanDriverEarning(
		r.db.QueryRow(
			ctx,
			query,
			tripID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get driver earning by trip id: %w",
			err,
		)
	}

	return earning, nil
}
