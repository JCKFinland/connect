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
