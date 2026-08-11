package presence

import "context"

func (s *Service) GoOffline(
	ctx context.Context,
	req GoOfflineRequest,
) error {

	return s.presence.SetOffline(
		ctx,
		req.UserID,
	)
}
