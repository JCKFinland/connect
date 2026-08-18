package ride_request

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByIDAuthorized returns a ride request only when the authenticated
// user has privileged access or owns the request.
func (s *Service) GetByIDAuthorized(
	ctx context.Context,
	id string,
	userID string,
) (*models.RideRequest, error) {

	if id == "" {
		return nil, fmt.Errorf("ride request ID is required")
	}

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	request, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return nil, err
	}

	roles, err := s.getUserRoles(
		ctx,
		userID,
	)
	if err != nil {
		return nil, err
	}

	if !canViewRideRequest(
		roles,
		userID,
		request,
	) {
		return nil, ErrRideRequestAccessDenied
	}

	return request, nil
}
