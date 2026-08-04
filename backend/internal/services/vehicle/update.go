package vehicle

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Update modifies an existing vehicle.
func (s *Service) Update(
	ctx context.Context,
	id string,
	req UpdateVehicleRequest,
) error {

	vehicle := &models.Vehicle{
		BaseModel: models.BaseModel{
			ID: id,
		},
		CompanyID:          req.CompanyID,
		BranchID:           req.BranchID,
		FleetID:            req.FleetID,
		RegistrationNumber: req.RegistrationNumber,
		VIN:                req.VIN,
		Make:               req.Make,
		Model:              req.Model,
		ModelYear:          req.ModelYear,
		Color:              req.Color,
		VehicleType:        req.VehicleType,
		SeatingCapacity:    req.SeatingCapacity,
		IsActive:           req.IsActive,
	}

	return s.repo.Update(
		ctx,
		vehicle,
	)
}