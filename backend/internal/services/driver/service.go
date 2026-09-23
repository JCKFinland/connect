package driver

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Dependencies contains the persistence dependencies required by the
// driver service.
type Dependencies struct {
	Drivers   repository.DriverRepository
	Companies repository.CompanyRepository
	Branches  repository.BranchRepository
}

type Service struct {
	repo      repository.DriverRepository
	companies repository.CompanyRepository
	branches  repository.BranchRepository
}

func NewService(deps Dependencies) *Service {
	return &Service{
		repo:      deps.Drivers,
		companies: deps.Companies,
		branches:  deps.Branches,
	}
}
