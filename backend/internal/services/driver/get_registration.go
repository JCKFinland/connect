package driver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// GetRegistration returns the driver registration belonging to the
// authenticated platform user.
func (s *Service) GetRegistration(
	ctx context.Context,
	user *models.User,
) (*models.Driver, error) {
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return nil, ErrInvalidDriver
	}

	if s.repo == nil {
		return nil, fmt.Errorf("driver repository is not configured")
	}

	driver, err := s.repo.GetByUserID(ctx, user.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDriverNotFound
		}

		return nil, fmt.Errorf("get driver registration: %w", err)
	}

	return driver, nil
}
