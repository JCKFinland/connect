package presence

import "context"

func (s *Service) UpdateAvailability(
	ctx context.Context,
	req UpdateAvailabilityRequest,
) error {

	return s.presence.UpdateAvailability(
		ctx,
		req.UserID,
		req.Status,
		true,
	)
}
