package driver

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// GetByID retrieves a driver through the authenticated caller's tenant boundary.
func (s *Service) GetByID(
	ctx context.Context,
	userID string,
	id string,
) (*models.Driver, error) {
	if s == nil ||
		s.repo == nil ||
		s.userRoles == nil ||
		userID == "" ||
		id == "" {
		return nil, ErrDriverNotFound
	}

	roles, err := s.userRoles.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve driver read authority: %w", err)
	}

	systemAdmin := false
	for _, role := range roles {
		if role == "SYSTEM_ADMIN" {
			systemAdmin = true
			break
		}
	}

	var driver *models.Driver

	if systemAdmin {
		driver, err = s.repo.GetByID(ctx, id)
	} else {
		driver, err = s.repo.GetByIDForCompanyMember(
			ctx,
			userID,
			id,
		)
	}

	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrDriverNotFound
	}
	if err != nil {
		return nil, err
	}

	return driver, nil
}
