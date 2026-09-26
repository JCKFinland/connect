package vehicle

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Create registers a new vehicle.
func (s *Service) Create(
	ctx context.Context,
	req CreateVehicleRequest,
) (*VehicleResponse, error) {

	vehicle := &models.Vehicle{
		CompanyID:          req.CompanyID,
		BranchID:           req.BranchID,
		FleetID:            req.FleetID,
		RegistrationNumber: req.RegistrationNumber,
		VIN:                normalizeVIN(req.VIN),
		Make:               req.Make,
		Model:              req.Model,
		ModelYear:          req.ModelYear,
		Color:              req.Color,
		VehicleType:        req.VehicleType,
		FuelType:           req.FuelType,
		SeatingCapacity:    req.SeatingCapacity,
		IsActive:           req.IsActive,
	}

	if err := s.vehicles.Create(
		ctx,
		vehicle,
	); err != nil {
		return nil, err
	}

	return &VehicleResponse{
		ID:                 vehicle.ID,
		CompanyID:          vehicle.CompanyID,
		BranchID:           vehicle.BranchID,
		FleetID:            vehicle.FleetID,
		RegistrationNumber: vehicle.RegistrationNumber,
		VIN:                vinValue(vehicle.VIN),
		Make:               vehicle.Make,
		Model:              vehicle.Model,
		ModelYear:          vehicle.ModelYear,
		Color:              vehicle.Color,
		VehicleType:        vehicle.VehicleType,
		FuelType:           vehicle.FuelType,
		SeatingCapacity:    vehicle.SeatingCapacity,
		IsActive:           vehicle.IsActive,
	}, nil
}
