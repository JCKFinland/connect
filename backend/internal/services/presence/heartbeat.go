package presence

import "context"

func (s *Service) Heartbeat(
	ctx context.Context,
	req HeartbeatRequest,
) error {

	return s.presence.UpdateHeartbeat(
		ctx,
		req.UserID,
		req.Latitude,
		req.Longitude,
		req.Heading,
		req.Speed,
		req.Accuracy,
	)
}
