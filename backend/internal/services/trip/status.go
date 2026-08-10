package trip

import (
	"context"
	"fmt"
)

// Valid trip lifecycle statuses.
const (
	StatusAssigned       = "ASSIGNED"
	StatusDriverEnRoute  = "DRIVER_EN_ROUTE"
	StatusDriverArrived  = "DRIVER_ARRIVED"
	StatusInProgress     = "IN_PROGRESS"
	StatusCompleted      = "COMPLETED"
	StatusCancelled      = "CANCELLED"
)

// UpdateStatus validates and applies a trip lifecycle transition.
func (s *tripService) UpdateStatus(
	ctx context.Context,
	id string,
	newStatus string,
) error {
	if id == "" {
		return fmt.Errorf("trip ID is required")
	}

	if !isValidStatus(newStatus) {
		return fmt.Errorf("invalid trip status: %s", newStatus)
	}

	trip, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get trip for status update: %w", err)
	}

	if err := validateStatusTransition(trip.Status, newStatus); err != nil {
		return err
	}

	return s.repo.UpdateStatus(ctx, id, newStatus)
}

func isValidStatus(status string) bool {
	switch status {
	case StatusAssigned,
		StatusDriverEnRoute,
		StatusDriverArrived,
		StatusInProgress,
		StatusCompleted,
		StatusCancelled:
		return true
	default:
		return false
	}
}

func validateStatusTransition(
	currentStatus string,
	newStatus string,
) error {
	if currentStatus == newStatus {
		return fmt.Errorf(
			"trip is already in status %s",
			currentStatus,
		)
	}

	switch currentStatus {

	case StatusAssigned:
		switch newStatus {
		case StatusDriverEnRoute, StatusCancelled:
			return nil
		}

	case StatusDriverEnRoute:
		switch newStatus {
		case StatusDriverArrived, StatusCancelled:
			return nil
		}

	case StatusDriverArrived:
		switch newStatus {
		case StatusInProgress, StatusCancelled:
			return nil
		}

	case StatusInProgress:
		switch newStatus {
		case StatusCompleted, StatusCancelled:
			return nil
		}

	case StatusCompleted:
		// Completed is terminal.

	case StatusCancelled:
		// Cancelled is terminal.
	}

	return fmt.Errorf(
		"invalid trip status transition: %s -> %s",
		currentStatus,
		newStatus,
	)
}