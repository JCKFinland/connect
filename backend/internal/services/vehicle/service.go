package vehicle

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Dependencies struct {
	Vehicles repository.VehicleRepository
	Drivers  repository.DriverRepository
	Fleets   repository.FleetRepository
}

type Service struct {
	vehicles repository.VehicleRepository
	drivers  repository.DriverRepository
	fleets   repository.FleetRepository
}

func NewService(
	deps Dependencies,
) *Service {

	return &Service{
		vehicles: deps.Vehicles,
		drivers:  deps.Drivers,
		fleets:   deps.Fleets,
	}
}
