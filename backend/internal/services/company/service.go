package company

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	Config             *config.Config
	DB                 *pgxpool.Pool
	Companies          repository.CompanyRepository
	UserRoles          repository.UserRoleRepository
	CompanyMemberships repository.CompanyMembershipRepository
}

type Service struct {
	cfg                *config.Config
	db                 *pgxpool.Pool
	companies          repository.CompanyRepository
	userRoles          repository.UserRoleRepository
	companyMemberships repository.CompanyMembershipRepository
}

func NewService(
	deps Dependencies,
) *Service {
	return &Service{
		cfg:                deps.Config,
		db:                 deps.DB,
		companies:          deps.Companies,
		userRoles:          deps.UserRoles,
		companyMemberships: deps.CompanyMemberships,
	}
}
