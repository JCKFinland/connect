package driver

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List returns all active (non-deleted) drivers.
func (s *Service) List(
	ctx context.Context,
) ([]models.Driver, error) {

	drivers, err := s.repo.List(
		ctx,
	)
	if err != nil {
		return nil, err
	}

	return drivers, nil
}