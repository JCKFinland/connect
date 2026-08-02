package company

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Dependencies struct {
	Config *config.Config

	Companies repository.CompanyRepository
}

type Service struct {
	cfg *config.Config

	companies repository.CompanyRepository
}

func NewService(
	deps Dependencies,
) *Service {

	return &Service{
		cfg: deps.Config,

		companies: deps.Companies,
	}
}