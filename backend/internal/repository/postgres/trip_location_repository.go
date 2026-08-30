package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TripLocationRepository implements repository.TripLocationRepository.
type TripLocationRepository struct {
	db DBTX
}

// Compile-time interface check.
var _ repository.TripLocationRepository = (*TripLocationRepository)(nil)

// NewTripLocationRepository creates a PostgreSQL trip location repository.
func NewTripLocationRepository(
	db *pgxpool.Pool,
) *TripLocationRepository {
	return &TripLocationRepository{
		db: db,
	}
}

// NewTripLocationRepositoryWithDB creates a trip location repository using
// either the connection pool or an active transaction.
func NewTripLocationRepositoryWithDB(
	db DBTX,
) *TripLocationRepository {
	return &TripLocationRepository{
		db: db,
	}
}

// Create stores an immutable trip location sample.
func (r *TripLocationRepository) Create(
	ctx context.Context,
	location *models.TripLocation,
) error {

	if location == nil {
		return fmt.Errorf("trip location is required")
	}

	if location.ID == "" {
		location.ID = uuid.NewString()
	}

	if location.RecordedAt.IsZero() {
		location.RecordedAt = time.Now().UTC()
	}

	const query = `
		INSERT INTO trip_locations
		(
			id,
			trip_id,
			driver_id,
			latitude,
			longitude,
			altitude,
			speed_kmh,
			heading,
			accuracy_meters,
			recorded_at
		)
		VALUES
		(
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10
		)
		RETURNING recorded_at
	`

	if err := r.db.QueryRow(
		ctx,
		query,
		location.ID,
		location.TripID,
		location.DriverID,
		location.Latitude,
		location.Longitude,
		location.Altitude,
		location.SpeedKMH,
		location.Heading,
		location.AccuracyMeters,
		location.RecordedAt,
	).Scan(
		&location.RecordedAt,
	); err != nil {
		return fmt.Errorf(
			"create trip location: %w",
			err,
		)
	}

	return nil
}

// ListByTripID returns all trip location samples in chronological order.
func (r *TripLocationRepository) ListByTripID(
	ctx context.Context,
	tripID string,
) ([]*models.TripLocation, error) {

	const query = `
		SELECT
			id,
			trip_id,
			driver_id,
			latitude,
			longitude,
			altitude,
			speed_kmh,
			heading,
			accuracy_meters,
			recorded_at
		FROM trip_locations
		WHERE trip_id = $1
		ORDER BY recorded_at ASC, id ASC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		tripID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list trip locations: %w",
			err,
		)
	}
	defer rows.Close()

	locations := make([]*models.TripLocation, 0)

	for rows.Next() {
		location := &models.TripLocation{}

		if err := rows.Scan(
			&location.ID,
			&location.TripID,
			&location.DriverID,
			&location.Latitude,
			&location.Longitude,
			&location.Altitude,
			&location.SpeedKMH,
			&location.Heading,
			&location.AccuracyMeters,
			&location.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf(
				"scan trip location: %w",
				err,
			)
		}

		locations = append(
			locations,
			location,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate trip locations: %w",
			err,
		)
	}

	return locations, nil
}
