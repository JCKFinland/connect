package ride_request

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// ListAuthorized returns ride requests scoped to the authenticated user.
//
// Privileged operational roles may use the supplied customer filter.
// Customers are always restricted to their own ride requests.
func (s *Service) ListAuthorized(
	ctx context.Context,
	userID string,
	customerID string,
	status string,
	limit int,
	offset int,
) ([]*models.RideRequest, error) {

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	roles, err := s.getUserRoles(
		ctx,
		userID,
	)
	if err != nil {
		return nil, err
	}

	if isPrivilegedRideRequestRole(roles) {
		return s.repo.List(
			ctx,
			customerID,
			status,
			limit,
			offset,
		)
	}

	isCustomer := false

	for _, role := range roles {
		if role == "CUSTOMER" {
			isCustomer = true
			break
		}
	}

	if !isCustomer {
		return nil, ErrRideRequestAccessDenied
	}

	// Ignore any customer_id supplied by the caller.
	// Customers may list only their own ride requests.
	return s.repo.List(
		ctx,
		userID,
		status,
		limit,
		offset,
	)
}
