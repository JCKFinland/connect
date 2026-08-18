package ride_request

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrRideRequestStatusAccessDenied = errors.New(
		"ride request status access denied",
	)

	ErrInvalidRideRequestStatusTransition = errors.New(
		"invalid ride request status transition",
	)
)

const (
	StatusPending   = "PENDING"
	StatusMatching  = "MATCHING"
	StatusAccepted  = "ACCEPTED"
	StatusCancelled = "CANCELLED"
	StatusExpired   = "EXPIRED"
)

// UpdateStatus changes the status of a ride request.
// This remains the low-level internal primitive.
func (s *Service) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) error {

	return s.repo.UpdateStatus(
		ctx,
		id,
		status,
	)
}

// UpdateStatusAuthorized validates ownership, role permissions,
// and lifecycle transitions before changing ride-request status.
func (s *Service) UpdateStatusAuthorized(
	ctx context.Context,
	id string,
	newStatus string,
	userID string,
) error {

	if id == "" {
		return fmt.Errorf("ride request ID is required")
	}

	if userID == "" {
		return fmt.Errorf("user ID is required")
	}

	current, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return err
	}

	roles, err := s.getUserRoles(
		ctx,
		userID,
	)
	if err != nil {
		return err
	}

	if isPrivilegedRideRequestRole(roles) {
		if err := validatePrivilegedStatusTransition(
			current.Status,
			newStatus,
		); err != nil {
			return err
		}

		return s.repo.UpdateStatus(
			ctx,
			id,
			newStatus,
		)
	}

	if current.CustomerID != userID {
		return ErrRideRequestStatusAccessDenied
	}

	if newStatus != StatusCancelled {
		return ErrRideRequestStatusAccessDenied
	}

	if err := validateCustomerCancellation(
		current.Status,
	); err != nil {
		return err
	}

	return s.repo.UpdateStatus(
		ctx,
		id,
		StatusCancelled,
	)
}

func validatePrivilegedStatusTransition(
	currentStatus string,
	newStatus string,
) error {

	if currentStatus == newStatus {
		return fmt.Errorf(
			"%w: request already has status %s",
			ErrInvalidRideRequestStatusTransition,
			currentStatus,
		)
	}

	switch currentStatus {

	case StatusPending:
		switch newStatus {
		case StatusMatching,
			StatusCancelled,
			StatusExpired:
			return nil
		}

	case StatusMatching:
		switch newStatus {
		case StatusAccepted,
			StatusCancelled,
			StatusExpired:
			return nil
		}

	case StatusAccepted:
		switch newStatus {
		case StatusCancelled:
			return nil
		}

	case StatusCancelled,
		StatusExpired:
		// Terminal.
	}

	return fmt.Errorf(
		"%w: %s -> %s",
		ErrInvalidRideRequestStatusTransition,
		currentStatus,
		newStatus,
	)
}

func validateCustomerCancellation(
	currentStatus string,
) error {

	switch currentStatus {

	case StatusPending,
		StatusMatching:
		return nil

	case StatusAccepted:
		return fmt.Errorf(
			"%w: accepted ride request cannot be cancelled through customer status endpoint",
			ErrInvalidRideRequestStatusTransition,
		)

	case StatusCancelled,
		StatusExpired:
		return fmt.Errorf(
			"%w: ride request is already terminal",
			ErrInvalidRideRequestStatusTransition,
		)

	default:
		return fmt.Errorf(
			"%w: unknown current status %s",
			ErrInvalidRideRequestStatusTransition,
			currentStatus,
		)
	}
}
