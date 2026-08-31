package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type TripMeterMeasurementRepository struct {
	db DBTX
}

func NewTripMeterMeasurementRepository(
	db DBTX,
) *TripMeterMeasurementRepository {
	return &TripMeterMeasurementRepository{
		db: db,
	}
}

func NewTripMeterMeasurementRepositoryWithDB(
	db DBTX,
) *TripMeterMeasurementRepository {
	return &TripMeterMeasurementRepository{
		db: db,
	}
}

func (r *TripMeterMeasurementRepository) Create(
	ctx context.Context,
	measurement *models.TripMeterMeasurement,
) error {
	if measurement == nil {
		return fmt.Errorf("trip meter measurement is required")
	}

	if measurement.ID == "" {
		measurement.ID = uuid.NewString()
	}

	if measurement.MeasuredAt.IsZero() {
		measurement.MeasuredAt = time.Now().UTC()
	}

	err := r.db.QueryRow(
		ctx,
		`
			INSERT INTO trip_meter_measurements (
				id,
				trip_id,
				measurement_source,
				algorithm_version,
				distance_meters,
				duration_seconds,
				waiting_duration_seconds,
				accepted_samples,
				rejected_samples,
				rejected_segments,
				measured_at
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				$9,
				$10,
				$11
			)
			RETURNING created_at
		`,
		measurement.ID,
		measurement.TripID,
		measurement.MeasurementSource,
		measurement.AlgorithmVersion,
		measurement.DistanceMeters,
		measurement.DurationSeconds,
		measurement.WaitingDurationSeconds,
		measurement.AcceptedSamples,
		measurement.RejectedSamples,
		measurement.RejectedSegments,
		measurement.MeasuredAt,
	).Scan(
		&measurement.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf(
			"create trip meter measurement: %w",
			err,
		)
	}

	return nil
}

func (r *TripMeterMeasurementRepository) GetByTripID(
	ctx context.Context,
	tripID string,
) (*models.TripMeterMeasurement, error) {
	measurement := &models.TripMeterMeasurement{}

	err := r.db.QueryRow(
		ctx,
		`
			SELECT
				id,
				trip_id,
				measurement_source,
				algorithm_version,
				distance_meters,
				duration_seconds,
				waiting_duration_seconds,
				accepted_samples,
				rejected_samples,
				rejected_segments,
				measured_at,
				created_at
			FROM trip_meter_measurements
			WHERE trip_id = $1
		`,
		tripID,
	).Scan(
		&measurement.ID,
		&measurement.TripID,
		&measurement.MeasurementSource,
		&measurement.AlgorithmVersion,
		&measurement.DistanceMeters,
		&measurement.DurationSeconds,
		&measurement.WaitingDurationSeconds,
		&measurement.AcceptedSamples,
		&measurement.RejectedSamples,
		&measurement.RejectedSegments,
		&measurement.MeasuredAt,
		&measurement.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get trip meter measurement by trip id: %w",
			err,
		)
	}

	return measurement, nil
}
