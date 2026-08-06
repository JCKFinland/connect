package driver_vehicle_assignment

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List returns all non-deleted driver-vehicle assignments.
func (s *Service) List(
	ctx context.Context,
) ([]models.DriverVehicleAssignment, error) {

	assignments, err := s.repo.List(
		ctx,
	)
	if err != nil {
		return nil, err
	}

	return assignments, nil
}