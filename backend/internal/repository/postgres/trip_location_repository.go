package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
// Create stores an immutable trip location sample.
//
// A physical GPS observation is uniquely identified by trip_id,
// driver_id, and recorded_at. Replaying the same observation is
// idempotent: PostgreSQL preserves the original immutable row and
// returns it instead of attempting a second insert.
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
		ON CONFLICT (
			trip_id,
			driver_id,
			recorded_at
		)
		DO NOTHING
		RETURNING
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
	`

	err := r.db.QueryRow(
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
	)

	if err == nil {
		return nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf(
			"create trip location: %w",
			err,
		)
	}

	// ON CONFLICT DO NOTHING returns no row. Recover the already
	// persisted immutable observation without aborting the transaction.
	existing, err := r.GetByObservationIdentity(
		ctx,
		location.TripID,
		location.DriverID,
		location.RecordedAt,
	)
	if err != nil {
		return fmt.Errorf(
			"recover existing trip location: %w",
			err,
		)
	}

	*location = *existing

	return nil
}

// GetByObservationIdentity returns a location sample by its immutable
// observation identity.
func (r *TripLocationRepository) GetByObservationIdentity(
	ctx context.Context,
	tripID string,
	driverID string,
	recordedAt time.Time,
) (*models.TripLocation, error) {
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
		  AND driver_id = $2
		  AND recorded_at = $3
		LIMIT 1
	`

	location := &models.TripLocation{}

	if err := r.db.QueryRow(
		ctx,
		query,
		tripID,
		driverID,
		recordedAt,
	).Scan(
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
			"get trip location by observation identity: %w",
			err,
		)
	}

	return location, nil
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
