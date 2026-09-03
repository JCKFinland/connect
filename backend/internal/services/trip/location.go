package trip

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
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

	if s.repo == nil {
		return nil, errors.New(
			"trip repository is not configured",
		)
	}

	if s.userRoles == nil {
		return nil, errors.New(
			"user role repository is not configured",
		)
	}

	if s.tripLocations == nil {
		return nil, errors.New(
			"trip location repository is not configured",
		)
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

	currentTrip, err := s.repo.GetByID(
		ctx,
		tripID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get trip for location recording: %w",
			err,
		)
	}

	if currentTrip.DriverID != actorUserID {
		return nil, ErrTripLocationAccessDenied
	}

	if currentTrip.Status != StatusInProgress {
		return nil, ErrTripNotInProgress
	}

	if err := validateRecordLocationRequest(req); err != nil {
		return nil, err
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

	if err := s.tripLocations.Create(
		ctx,
		location,
	); err != nil {
		return nil, fmt.Errorf(
			"record trip location: %w",
			err,
		)
	}

	return location, nil
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
