package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// ListAuthorized returns only trips the authenticated user is permitted to see.
//
// Privileged operational roles may use the supplied filters.
// Drivers are always scoped to their own assigned trips.
// Customers are always scoped to their own customer trips.
func (s *tripService) ListAuthorized(
	ctx context.Context,
	userID string,
	companyID string,
	branchID string,
	status string,
	driverID string,
	customerID string,
	limit int,
	offset int,
) ([]*models.Trip, error) {

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	if limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}

	if offset < 0 {
		return nil, fmt.Errorf("offset cannot be negative")
	}

	if limit == 0 {
		limit = 50
	}

	if limit > 100 {
		limit = 100
	}

	roles, err := s.userRoles.GetUserRoles(
		ctx,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get user roles: %w",
			err,
		)
	}

	var (
		privileged bool
		isDriver   bool
		isCustomer bool
	)

	for _, role := range roles {
		switch role {

		case "SYSTEM_ADMIN",
			"COMPANY_ADMIN",
			"DISPATCHER":
			privileged = true

		case "DRIVER":
			isDriver = true

		case "CUSTOMER":
			isCustomer = true
		}
	}

	// Privileged users retain the filters supplied by the API caller.
	if privileged {
		return s.repo.List(
			ctx,
			companyID,
			branchID,
			status,
			driverID,
			customerID,
			limit,
			offset,
		)
	}

	// DRIVER takes precedence over CUSTOMER for this operational endpoint.
	if isDriver {
		return s.repo.List(
			ctx,
			companyID,
			branchID,
			status,
			userID,
			"",
			limit,
			offset,
		)
	}

	if isCustomer {
		return s.repo.List(
			ctx,
			companyID,
			branchID,
			status,
			"",
			userID,
			limit,
			offset,
		)
	}

	return nil, ErrTripAccessDenied
}
