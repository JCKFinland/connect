package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

var ErrActiveTripNotFound = errors.New(
	"active driver trip not found",
)

// GetActiveByDriver returns the authenticated driver's current non-terminal trip.
func (s *tripService) GetActiveByDriver(
	ctx context.Context,
	driverUserID string,
) (*models.Trip, error) {
	if driverUserID == "" {
		return nil, fmt.Errorf("driver user ID is required")
	}

	roles, err := s.userRoles.GetUserRoles(
		ctx,
		driverUserID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get user roles for active driver trip: %w",
			err,
		)
	}

	isDriver := false

	for _, role := range roles {
		if role == "DRIVER" {
			isDriver = true
			break
		}
	}

	if !isDriver {
		return nil, ErrTripAccessDenied
	}

	currentTrip, err := s.repo.GetActiveByDriverID(
		ctx,
		driverUserID,
	)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrActiveTripNotFound
	}
	if err != nil {
		return nil, fmt.Errorf(
			"get active trip for driver: %w",
			err,
		)
	}

	return currentTrip, nil
}
