package driver_vehicle_assignment

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByID returns a driver-vehicle assignment by its ID.
func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*models.DriverVehicleAssignment, error) {

	assignment, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return nil, err
	}

	return assignment, nil
}