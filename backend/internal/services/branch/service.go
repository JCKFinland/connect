package branch

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	Config             *config.Config
	DB                 *pgxpool.Pool
	Branches           repository.BranchRepository
	UserRoles          repository.UserRoleRepository
	CompanyMemberships repository.CompanyMembershipRepository
}

type Service struct {
	config             *config.Config
	db                 *pgxpool.Pool
	branches           repository.BranchRepository
	userRoles          repository.UserRoleRepository
	companyMemberships repository.CompanyMembershipRepository
}

func NewService(
	deps Dependencies,
) *Service {
	return &Service{
		config:             deps.Config,
		db:                 deps.DB,
		branches:           deps.Branches,
		userRoles:          deps.UserRoles,
		companyMemberships: deps.CompanyMemberships,
	}
}
