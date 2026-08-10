package trip

import (
	"context"
	"fmt"
)

// Delete soft-deletes a trip.
func (s *tripService) Delete(
	ctx context.Context,
	id string,
) error {
	if id == "" {
		return fmt.Errorf("trip ID is required")
	}

	return s.repo.Delete(ctx, id)
}
