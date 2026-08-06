package driver_vehicle_assignment

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Service struct {
	repo repository.DriverVehicleAssignmentRepository
}

func NewService(
	repo repository.DriverVehicleAssignmentRepository,
) *Service {

	return &Service{
		repo: repo,
	}
}