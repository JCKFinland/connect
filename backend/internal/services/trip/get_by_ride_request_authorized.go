package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByRideRequestIDAuthorized resolves a trip only after the caller
// has been authorized to view its originating ride request.
func (s *tripService) GetByRideRequestIDAuthorized(
	ctx context.Context,
	rideRequestID string,
	userID string,
) (*models.Trip, error) {
	if rideRequestID == "" {
		return nil, fmt.Errorf("ride request ID is required")
	}

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	if s.rideRequestAccess == nil {
		return nil, fmt.Errorf("ride request authorization is not configured")
	}

	if _, err := s.rideRequestAccess.GetByIDAuthorized(
		ctx,
		rideRequestID,
		userID,
	); err != nil {
		return nil, err
	}

	tripID, err := s.repo.GetIDByRideRequestID(ctx, rideRequestID)
	if err != nil {
		return nil, err
	}

	result, err := s.GetByIDAuthorized(ctx, tripID, userID)
	if err != nil {
		return nil, err
	}

	if result.RideRequestID != rideRequestID {
		return nil, fmt.Errorf(
			"trip ride request relationship mismatch",
		)
	}

	return result, nil
}
