package driver_vehicle_assignment

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Assign creates a new operational driver-vehicle assignment.
func (s *Service) Assign(
	ctx context.Context,
	req AssignDriverVehicleRequest,
) (*models.DriverVehicleAssignment, error) {

	now := time.Now().UTC()

	assignment := &models.DriverVehicleAssignment{
		BaseModel: models.BaseModel{
			ID:        uuid.NewString(),
			CreatedAt: now,
			UpdatedAt: now,
		},

		CompanyID: req.CompanyID,
		BranchID:  req.BranchID,
		FleetID:   req.FleetID,

		DriverID:  req.DriverID,
		VehicleID: req.VehicleID,

		Status: "ACTIVE",

		AssignedAt: now,

		AssignedBy: req.AssignedBy,

		Notes: req.Notes,

		IsActive: true,
	}

	if err := s.repo.Assign(
		ctx,
		assignment,
	); err != nil {
		return nil, err
	}

	return assignment, nil
}