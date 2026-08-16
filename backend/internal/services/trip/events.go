package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

var ErrTripEventAccessDenied = errors.New(
	"trip event access denied",
)

func (s *tripService) ListEvents(
	ctx context.Context,
	tripID string,
	userID string,
) ([]*models.TripEvent, error) {

	if tripID == "" {
		return nil, fmt.Errorf("trip ID is required")
	}

	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	trip, err := s.repo.GetByID(
		ctx,
		tripID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get trip before listing events: %w",
			err,
		)
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

	if !canViewTripEvents(
		roles,
		userID,
		trip,
	) {
		return nil, ErrTripEventAccessDenied
	}

	events, err := s.tripEvents.ListByTripID(
		ctx,
		tripID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list trip events: %w",
			err,
		)
	}

	return events, nil
}

func canViewTripEvents(
	roles []string,
	userID string,
	trip *models.Trip,
) bool {

	if trip == nil {
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

	if trip.DriverID == userID {
		return true
	}

	if trip.CustomerID == userID {
		return true
	}

	return false
}
