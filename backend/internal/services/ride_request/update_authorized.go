package ride_request

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

var ErrRideRequestNotEditable = errors.New(
	"ride request is not editable in its current status",
)

// UpdateAuthorized updates a ride request only when the authenticated
// user has permission to modify it.
//
// Privileged operational users may update requests they manage.
// Customers may update only their own requests while status is PENDING.
func (s *Service) UpdateAuthorized(
	ctx context.Context,
	request *models.RideRequest,
	userID string,
) error {

	if request == nil {
		return fmt.Errorf("ride request is required")
	}

	if request.ID == "" {
		return fmt.Errorf("ride request ID is required")
	}

	if userID == "" {
		return fmt.Errorf("user ID is required")
	}

	current, err := s.repo.GetByID(
		ctx,
		request.ID,
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
		return s.repo.Update(
			ctx,
			request,
		)
	}

	if current.CustomerID != userID {
		return ErrRideRequestAccessDenied
	}

	if current.Status != "PENDING" {
		return ErrRideRequestNotEditable
	}

	// Preserve protected ownership/lifecycle fields.
	request.CustomerID = current.CustomerID
	request.Status = current.Status
	request.RequestedAt = current.RequestedAt

	return s.repo.Update(
		ctx,
		request,
	)
}
