package vehicle

import (
	"context"
)

// GetByID returns a vehicle by its unique identifier.
func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*VehicleResponse, error) {

	vehicle, err := s.repo.GetByID(
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
		VIN:                vehicle.VIN,
		Make:               vehicle.Make,
		Model:              vehicle.Model,
		ModelYear:          vehicle.ModelYear,
		Color:              vehicle.Color,
		VehicleType:        vehicle.VehicleType,
		SeatingCapacity:    vehicle.SeatingCapacity,
		IsActive:           vehicle.IsActive,
	}, nil
}