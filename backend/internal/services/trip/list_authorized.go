package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// tenantTripLister provides SQL-enforced company membership filtering.
type tenantTripLister interface {
	ListForCompanyMember(
		ctx context.Context,
		userID string,
		companyID string,
		branchID string,
		status string,
		driverID string,
		customerID string,
		limit int,
		offset int,
	) ([]*models.Trip, error)
}

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
		isSystemAdmin bool
		isOperational bool
		isDriver      bool
		isCustomer    bool
	)

	for _, role := range roles {
		switch role {
		case "SYSTEM_ADMIN":
			isSystemAdmin = true

		case "COMPANY_ADMIN", "DISPATCHER":
			isOperational = true

		case "DRIVER":
			isDriver = true

		case "CUSTOMER":
			isCustomer = true
		}
	}

	// SYSTEM_ADMIN retains unrestricted administrative visibility.
	if isSystemAdmin {
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

	// Operational staff must be scoped by canonical company membership
	// inside SQL, before ordering and pagination.
	if isOperational {
		if s.db == nil {
			return nil, fmt.Errorf(
				"trip database is not configured",
			)
		}
		if s.companyMemberships == nil {
			return nil, fmt.Errorf(
				"trip company membership repository is not configured",
			)
		}

		trips, ok := s.repo.(tenantTripLister)
		if !ok {
			return nil, fmt.Errorf(
				"trip repository does not support tenant-authorized listing",
			)
		}

		return trips.ListForCompanyMember(
			ctx,
			userID,
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
