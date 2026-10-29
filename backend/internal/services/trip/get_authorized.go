package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

var ErrTripAccessDenied = errors.New(
	"trip access denied",
)

// GetByIDAuthorized returns a trip only when the requesting user
// has privileged operational access or owns/is assigned to the trip.
func (s *tripService) GetByIDAuthorized(
	ctx context.Context,
	tripID string,
	userID string,
) (*models.Trip, error) {

	if tripID == "" {
		return nil, fmt.Errorf("trip ID is required")
	}

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	result, err := s.repo.GetByID(
		ctx,
		tripID,
	)
	if err != nil {
		return nil, err
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

	allowed, err := s.canViewTripAuthorized(
		ctx,
		roles,
		userID,
		result,
	)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrTripAccessDenied
	}

	return result, nil
}

// canViewTrip determines whether a user may read a specific trip.
func canViewTrip(
	roles []string,
	userID string,
	result *models.Trip,
) bool {

	if result == nil {
		return false
	}

	for _, role := range roles {
		switch role {

		case "SYSTEM_ADMIN",
			"COMPANY_ADMIN",
			"DISPATCHER":

			return true
		}
	}

	// The assigned driver may view the trip.
	if result.DriverID == userID {
		return true
	}

	// The owning customer may view the trip.
	if result.CustomerID == userID {
		return true
	}

	return false
}

// canViewTripAuthorized preserves customer and assigned-driver access
// while restricting operational roles to their company membership.
func (s *tripService) canViewTripAuthorized(
	ctx context.Context,
	roles []string,
	userID string,
	result *models.Trip,
) (bool, error) {
	if result == nil || userID == "" {
		return false, nil
	}

	if result.DriverID == userID || result.CustomerID == userID {
		return true, nil
	}

	return s.hasOperationalCompanyAuthority(
		ctx,
		userID,
		result.CompanyID,
		roles,
	)
}
