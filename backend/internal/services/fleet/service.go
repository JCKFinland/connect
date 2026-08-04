package fleet

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Service struct {
	repo repository.FleetRepository
}

func NewService(
	repo repository.FleetRepository,
) *Service {

	return &Service{
		repo: repo,
	}
}