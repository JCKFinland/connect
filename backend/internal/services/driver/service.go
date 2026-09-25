package driver

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Dependencies contains the persistence dependencies required by the
// driver service.
type Dependencies struct {
	DB        *pgxpool.Pool
	Drivers   repository.DriverRepository
	Companies repository.CompanyRepository
	Branches  repository.BranchRepository
}

type Service struct {
	db        *pgxpool.Pool
	repo      repository.DriverRepository
	companies repository.CompanyRepository
	branches  repository.BranchRepository
}

func NewService(deps Dependencies) *Service {
	return &Service{
		db:        deps.DB,
		repo:      deps.Drivers,
		companies: deps.Companies,
		branches:  deps.Branches,
	}
}
