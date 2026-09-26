package vehicle

import (
	"context"
)

// GetByID returns a vehicle by its unique identifier.
func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*VehicleResponse, error) {

	vehicle, err := s.vehicles.GetByID(
		ctx,
		id,
	)
	if err != nil {
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
