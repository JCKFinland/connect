package vehicle

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	DB                 *pgxpool.Pool
	Vehicles           repository.VehicleRepository
	Drivers            repository.DriverRepository
	Fleets             repository.FleetRepository
	UserRoles          repository.UserRoleRepository
	CompanyMemberships repository.CompanyMembershipRepository
}

type Service struct {
	db                 *pgxpool.Pool
	vehicles           repository.VehicleRepository
	drivers            repository.DriverRepository
	fleets             repository.FleetRepository
	userRoles          repository.UserRoleRepository
	companyMemberships repository.CompanyMembershipRepository
}

func NewService(
	deps Dependencies,
) *Service {

	return &Service{
		db:                 deps.DB,
		vehicles:           deps.Vehicles,
		drivers:            deps.Drivers,
		fleets:             deps.Fleets,
		userRoles:          deps.UserRoles,
		companyMemberships: deps.CompanyMemberships,
	}
}
