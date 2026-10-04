package fleet

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	DB                 *pgxpool.Pool
	Fleets             repository.FleetRepository
	Drivers            repository.DriverRepository
	Branches           repository.BranchRepository
	UserRoles          repository.UserRoleRepository
	CompanyMemberships repository.CompanyMembershipRepository
}

type Service struct {
	db                 *pgxpool.Pool
	fleets             repository.FleetRepository
	drivers            repository.DriverRepository
	branches           repository.BranchRepository
	userRoles          repository.UserRoleRepository
	companyMemberships repository.CompanyMembershipRepository
}

func NewService(deps Dependencies) *Service {
	return &Service{
		db:                 deps.DB,
		fleets:             deps.Fleets,
		drivers:            deps.Drivers,
		branches:           deps.Branches,
		userRoles:          deps.UserRoles,
		companyMemberships: deps.CompanyMemberships,
	}
}
