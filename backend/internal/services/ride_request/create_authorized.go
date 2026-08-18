package ride_request

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// CreateAuthorized creates a ride request within the authenticated user's
// authorization scope.
//
// Privileged operational users may create a request on behalf of another
// customer. Ordinary customers are always forced to create requests for
// themselves, regardless of the customer_id supplied by the client.
func (s *Service) CreateAuthorized(
	ctx context.Context,
	req CreateRideRequestRequest,
	userID string,
) (*models.RideRequest, error) {

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

	// Operational users may create ride requests on behalf of customers.
	if isPrivilegedRideRequestRole(roles) {
		if req.CustomerID == "" {
			return nil, fmt.Errorf("customer ID is required")
		}

		return s.Create(
			ctx,
			req,
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

	// Never trust customer_id supplied by an ordinary customer.
	// Force ownership to the authenticated account.
	req.CustomerID = userID

	return s.Create(
		ctx,
		req,
	)
}
