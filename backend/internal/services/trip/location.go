package trip

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

const (
	maximumLocationAccuracyMeters = 50.0
	maximumLocationSpeedKMH       = 180.0

	maximumLocationAge        = 5 * time.Minute
	maximumLocationFutureSkew = 30 * time.Second
)

func (s *tripService) RecordTripLocation(
	ctx context.Context,
	tripID string,
	actorUserID string,
	req RecordLocationRequest,
) (*models.TripLocation, error) {
	if tripID == "" {
		return nil, fmt.Errorf(
			"%w: trip ID is required",
			ErrInvalidTripLocation,
		)
	}

	if actorUserID == "" {
		return nil, fmt.Errorf(
			"%w: authenticated user ID is required",
			ErrTripLocationAccessDenied,
		)
	}

	if s.db == nil {
		return nil, errors.New(
			"trip database is not configured",
		)
	}

	if s.userRoles == nil {
		return nil, errors.New(
			"user role repository is not configured",
		)
	}

	// Validate request data before taking the trip row lock.
	//
	// Invalid GPS data should never hold a lifecycle lock.
	if err := validateRecordLocationRequest(req); err != nil {
		return nil, err
	}

	roles, err := s.userRoles.GetUserRoles(
		ctx,
		actorUserID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get authenticated user roles: %w",
			err,
		)
	}

	isDriver := false

	for _, role := range roles {
		if role == "DRIVER" {
			isDriver = true
			break
		}
	}

	if !isDriver {
		return nil, ErrTripLocationAccessDenied
	}

	var recordedLocation *models.TripLocation

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			trips :=
				postgresrepo.NewTripRepositoryWithDB(tx)

			tripLocations :=
				postgresrepo.NewTripLocationRepositoryWithDB(tx)

			// -------------------------------------------------
			// Serialize GPS ingestion against trip completion.
			//
			// CompleteTrip locks this same trip row before it
			// reads the authoritative GPS evidence. Therefore:
			//
			//   1. a location committed before completion gets
			//      included in the final evidence set; or
			//   2. completion wins the lock, changes the trip to
			//      COMPLETED, and this request is rejected.
			//
			// No GPS sample can be committed after completion
			// based on a stale IN_PROGRESS read.
			// -------------------------------------------------

			currentTrip, err :=
				trips.GetByIDForUpdate(
					ctx,
					tripID,
				)
			if err != nil {
				return fmt.Errorf(
					"get trip for location recording: %w",
					err,
				)
			}

			if currentTrip.DriverID != actorUserID {
				return ErrTripLocationAccessDenied
			}

			if currentTrip.Status != StatusInProgress {
				return ErrTripNotInProgress
			}

			var heading *int16

			if req.Heading != nil {
				value := int16(*req.Heading)
				heading = &value
			}

			location := &models.TripLocation{
				TripID:         currentTrip.ID,
				DriverID:       actorUserID,
				Latitude:       req.Latitude,
				Longitude:      req.Longitude,
				Altitude:       req.Altitude,
				SpeedKMH:       req.SpeedKMH,
				Heading:        heading,
				AccuracyMeters: req.AccuracyMeters,
				RecordedAt:     req.RecordedAt.UTC(),
			}

			if err := tripLocations.Create(
				ctx,
				location,
			); err != nil {
				return fmt.Errorf(
					"record trip location: %w",
					err,
				)
			}

			recordedLocation = location

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return recordedLocation, nil
}

func validateRecordLocationRequest(
	req RecordLocationRequest,
) error {
	if math.IsNaN(req.Latitude) ||
		math.IsInf(req.Latitude, 0) ||
		req.Latitude < -90 ||
		req.Latitude > 90 {

		return fmt.Errorf(
			"%w: latitude must be between -90 and 90",
			ErrInvalidTripLocation,
		)
	}

	if math.IsNaN(req.Longitude) ||
		math.IsInf(req.Longitude, 0) ||
		req.Longitude < -180 ||
		req.Longitude > 180 {

		return fmt.Errorf(
			"%w: longitude must be between -180 and 180",
			ErrInvalidTripLocation,
		)
	}

	if req.Altitude != nil {
		if math.IsNaN(*req.Altitude) ||
			math.IsInf(*req.Altitude, 0) {

			return fmt.Errorf(
				"%w: altitude must be finite",
				ErrInvalidTripLocation,
			)
		}
	}

	if req.SpeedKMH != nil {
		if math.IsNaN(*req.SpeedKMH) ||
			math.IsInf(*req.SpeedKMH, 0) ||
			*req.SpeedKMH < 0 ||
			*req.SpeedKMH > maximumLocationSpeedKMH {

			return fmt.Errorf(
				"%w: speed_kmh must be between 0 and %.0f",
				ErrInvalidTripLocation,
				maximumLocationSpeedKMH,
			)
		}
	}

	if req.Heading != nil {
		if *req.Heading < 0 || *req.Heading > 359 {
			return fmt.Errorf(
				"%w: heading must be between 0 and 359",
				ErrInvalidTripLocation,
			)
		}
	}

	if req.AccuracyMeters == nil {
		return fmt.Errorf(
			"%w: accuracy_meters is required",
			ErrInvalidTripLocation,
		)
	}

	if math.IsNaN(*req.AccuracyMeters) ||
		math.IsInf(*req.AccuracyMeters, 0) ||
		*req.AccuracyMeters < 0 ||
		*req.AccuracyMeters > maximumLocationAccuracyMeters {

		return fmt.Errorf(
			"%w: accuracy_meters must be between 0 and %.0f",
			ErrInvalidTripLocation,
			maximumLocationAccuracyMeters,
		)
	}

	if req.RecordedAt.IsZero() {
		return fmt.Errorf(
			"%w: recorded_at is required",
			ErrTripLocationTimestamp,
		)
	}

	now := time.Now().UTC()
	recordedAt := req.RecordedAt.UTC()

	if recordedAt.Before(now.Add(-maximumLocationAge)) {
		return fmt.Errorf(
			"%w: recorded_at is too old",
			ErrTripLocationTimestamp,
		)
	}

	if recordedAt.After(now.Add(maximumLocationFutureSkew)) {
		return fmt.Errorf(
			"%w: recorded_at is too far in the future",
			ErrTripLocationTimestamp,
		)
	}

	return nil
}
