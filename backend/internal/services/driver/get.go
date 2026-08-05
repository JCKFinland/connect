package driver

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByID retrieves a driver by its unique identifier.
func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*models.Driver, error) {

	driver, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return nil, err
	}

	return driver, nil
}