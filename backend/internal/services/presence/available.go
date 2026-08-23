package presence

import (
	"context"
	"errors"
)

var ErrDriverAvailabilityLocked = errors.New(
	"driver availability cannot be changed while committed to an active trip",
)

func (s *Service) UpdateAvailability(
	ctx context.Context,
	req UpdateAvailabilityRequest,
) error {

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
