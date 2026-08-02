package branch

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Dependencies struct {
	Config *config.Config

	Branches repository.BranchRepository
}

type Service struct {
	config *config.Config

	branches repository.BranchRepository
}

func NewService(
	deps Dependencies,
) *Service {

	return &Service{
		config: deps.Config,

		branches: deps.Branches,
	}
}