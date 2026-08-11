package presence

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

var ErrDriverNotFound = errors.New(
	"authenticated user is not registered as a driver",
)

func (s *Service) getDriverByUserID(
	ctx context.Context,
	userID string,
) (*models.Driver, error) {

	driver, err := s.drivers.GetByUserID(ctx, userID)

	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrDriverNotFound
	}

	if err != nil {
		return nil, err
	}

	if !driver.IsActive {
		return nil, errors.New("driver account is inactive")
	}

	return driver, nil
}
