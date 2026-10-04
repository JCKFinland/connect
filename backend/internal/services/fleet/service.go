package fleet

import "github.com/JCKFinland/connect/backend/internal/repository"

type Dependencies struct {
	Fleets             repository.FleetRepository
	Drivers            repository.DriverRepository
	Branches           repository.BranchRepository
	UserRoles          repository.UserRoleRepository
	CompanyMemberships repository.CompanyMembershipRepository
}

type Service struct {
	fleets             repository.FleetRepository
	drivers            repository.DriverRepository
	branches           repository.BranchRepository
	userRoles          repository.UserRoleRepository
	companyMemberships repository.CompanyMembershipRepository
}

func NewService(deps Dependencies) *Service {
	return &Service{
		fleets:             deps.Fleets,
		drivers:            deps.Drivers,
		branches:           deps.Branches,
		userRoles:          deps.UserRoles,
		companyMemberships: deps.CompanyMemberships,
	}
}
