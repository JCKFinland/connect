package presence

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

var ErrDriverHeartbeatUnavailable = errors.New(
	"driver heartbeat is unavailable while driver is offline",
)

var (
	ErrInvalidLatitude = errors.New(
		"latitude must be between -90 and 90",
	)

	ErrInvalidLongitude = errors.New(
		"longitude must be between -180 and 180",
	)

	ErrInvalidHeading = errors.New(
		"heading must be between 0 and 360",
	)

	ErrInvalidSpeed = errors.New(
		"speed must be zero or greater",
	)

	ErrInvalidAccuracy = errors.New(
		"accuracy must be zero or greater",
	)
)

func (s *Service) Heartbeat(
	ctx context.Context,
	req HeartbeatRequest,
) error {

	if s == nil {
		return errors.New(
			"presence service is required",
		)
	}

	if s.presence == nil {
		return errors.New(
			"driver presence repository is not configured",
		)
	}

	if req.UserID == "" {
		return errors.New(
			"user ID is required",
		)
	}

	// ---------------------------------------------------------
	// Validate telemetry before touching persistence.
	//
	// PostgreSQL CHECK constraints remain the final integrity
	// backstop, but malformed GPS data should be rejected at
	// the service boundary.
	// ---------------------------------------------------------

	if req.Latitude < -90 ||
		req.Latitude > 90 {

		return ErrInvalidLatitude
	}

	if req.Longitude < -180 ||
		req.Longitude > 180 {

		return ErrInvalidLongitude
	}

	if req.Heading < 0 ||
		req.Heading > 360 {

		return ErrInvalidHeading
	}

	if req.Speed < 0 {
		return ErrInvalidSpeed
	}

	if req.Accuracy < 0 {
		return ErrInvalidAccuracy
	}

	// ---------------------------------------------------------
	// 1. Confirm presence exists so missing presence can be
	//    distinguished from an offline/inactive presence row.
	// ---------------------------------------------------------

	_, err := s.presence.GetByDriverID(
		ctx,
		req.UserID,
	)

	if errors.Is(
		err,
		repository.ErrNotFound,
	) {
		return ErrDriverNotFound
	}

	if err != nil {
		return fmt.Errorf(
			"get driver presence before heartbeat: %w",
			err,
		)
	}

	// ---------------------------------------------------------
	// 2. Update live location only when the driver is in an
	//    online operational state.
	//
	// Valid:
	//   AVAILABLE
	//   BUSY
	//   BREAK
	//
	// Rejected:
	//   OFFLINE
	//   OFF_DUTY
	//   SUSPENDED
	// ---------------------------------------------------------

	updated, err :=
		s.presence.UpdateHeartbeatIfOnline(
			ctx,
			req.UserID,
			req.Latitude,
			req.Longitude,
			req.Heading,
			req.Speed,
			req.Accuracy,
		)

	if err != nil {
		return fmt.Errorf(
			"update driver heartbeat: %w",
			err,
		)
	}

	if !updated {
		return ErrDriverHeartbeatUnavailable
	}

	return nil
}
