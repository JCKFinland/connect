package presence

import "context"

func (s *Service) GoOffline(
	ctx context.Context,
	req GoOfflineRequest,
) error {

	updated, err :=
		s.presence.UpdateAvailabilityIfIdle(
			ctx,
			req.UserID,
			StatusOffline,
			false,
		)

	if err != nil {
		return err
	}

	if !updated {
		return ErrDriverAvailabilityLocked
	}

	return nil
}
