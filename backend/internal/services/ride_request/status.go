package ride_request

import "context"

// UpdateStatus changes the status of a ride request.
func (s *Service) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) error {
	return s.repo.UpdateStatus(
		ctx,
		id,
		status,
	)
}
