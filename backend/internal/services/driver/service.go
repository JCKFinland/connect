
package driver

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Service struct {
	repo repository.DriverRepository
}

func NewService(
	repo repository.DriverRepository,
) *Service {

	return &Service{
		repo: repo,
	}
}