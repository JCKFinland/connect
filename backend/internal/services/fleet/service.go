package fleet

import "github.com/JCKFinland/connect/backend/internal/repository"

type Dependencies struct {
	Fleets  repository.FleetRepository
	Drivers repository.DriverRepository
}

type Service struct {
	fleets  repository.FleetRepository
	drivers repository.DriverRepository
}

func NewService(deps Dependencies) *Service {
	return &Service{
		fleets:  deps.Fleets,
		drivers: deps.Drivers,
	}
}
