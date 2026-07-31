package assignment

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) Assign(
	ctx context.Context,
	req AssignDriverRequest,
) (*models.DriverAssignment, error) {

	// Driver already assigned?
	_, err := s.assignments.GetActiveByDriver(
		ctx,
		req.DriverID,
	)
	if err == nil {
		return nil, ErrDriverAlreadyAssigned
	}

	if err != repository.ErrNotFound {
		return nil, err
	}

	// Vehicle already assigned?
	_, err = s.assignments.GetActiveByVehicle(
		ctx,
		req.VehicleID,
	)
	if err == nil {
		return nil, ErrVehicleAlreadyAssigned
	}

	if err != repository.ErrNotFound {
		return nil, err
	}

	assignment := &models.DriverAssignment{
		CompanyID:  req.CompanyID,
		BranchID:   req.BranchID,
		FleetID:    req.FleetID,
		DriverID:   req.DriverID,
		VehicleID:  req.VehicleID,
		AssignedAt: time.Now().UTC(),
		Notes:      req.Notes,
	}

	if err := s.assignments.Create(
		ctx,
		assignment,
	); err != nil {
		return nil, err
	}


	if err := s.presence.AttachAssignment(
		ctx,
		assignment,
	); err != nil {
		return nil, err
	}

	return assignment, nil
}
