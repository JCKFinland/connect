package presence

import (
	"context"
	"errors"
)

var ErrDriverAvailabilityLocked = errors.New(
	"driver availability cannot be changed while committed to an active trip",
)

var ErrInvalidAvailabilityStatus = errors.New(
	"invalid driver availability status",
)

func (s *Service) UpdateAvailability(
	ctx context.Context,
	req UpdateAvailabilityRequest,
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

	switch req.Status {
	case StatusAvailable,
		StatusBreak,
		StatusOffDuty:

		// Driver-controlled availability states.

	default:
		return ErrInvalidAvailabilityStatus
	}

	updated, err :=
		s.presence.UpdateAvailabilityIfIdle(
			ctx,
			req.UserID,
			req.Status,
			true,
		)

	if err != nil {
		return err
	}

	if !updated {
		return ErrDriverAvailabilityLocked
	}

	return nil
}
