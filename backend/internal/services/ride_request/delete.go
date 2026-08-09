package ride_request

import "context"

// Delete removes a ride request.
func (s *Service) Delete(
	ctx context.Context,
	id string,
) error {
	return s.repo.Delete(
		ctx,
		id,
	)
}
