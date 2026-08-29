package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5"
)

func (r *TripFareRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.TripFare, error) {
	query := `
		SELECT
			` + tripFareColumns + `
		FROM trip_fares
		WHERE id = $1
	`

	fare, err := scanTripFare(
		r.db.QueryRow(ctx, query, id),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("get trip fare by id: %w", err)
	}

	return fare, nil
}

func (r *TripFareRepository) GetByTripID(
	ctx context.Context,
	tripID string,
) (*models.TripFare, error) {
	query := `
		SELECT
			` + tripFareColumns + `
		FROM trip_fares
		WHERE trip_id = $1
	`

	fare, err := scanTripFare(
		r.db.QueryRow(ctx, query, tripID),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get trip fare by trip id: %w",
			err,
		)
	}

	return fare, nil
}
