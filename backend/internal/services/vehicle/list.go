package vehicle

import (
	"context"
)

// List returns all active vehicles.
func (s *Service) List(
	ctx context.Context,
) ([]VehicleResponse, error) {

	vehicles, err := s.repo.List(
		ctx,
	)
	if err != nil {
		return nil, err
	}

	response := make([]VehicleResponse, 0, len(vehicles))

	for _, vehicle := range vehicles {

		response = append(response, VehicleResponse{
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
		})
	}

	return response, nil
}