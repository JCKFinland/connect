package vehicle

import (
	"context"
)

// Delete performs a soft delete.
func (s *Service) Delete(
	ctx context.Context,
	id string,
) error {

	return s.repo.Delete(
		ctx,
		id,
	)
}