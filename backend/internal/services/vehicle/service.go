package vehicle

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Service struct {
	repo repository.VehicleRepository
}

func NewService(
	repo repository.VehicleRepository,
) *Service {

	return &Service{
		repo: repo,
	}
}