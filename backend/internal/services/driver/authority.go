package driver

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// resolveManagementTarget resolves a driver through the authenticated caller's
// tenant authority boundary.
//
// SYSTEM_ADMIN has global authority. All other callers must be explicit members
// of the driver's current company. Unauthorized targets are intentionally
// indistinguishable from nonexistent drivers.
func (s *Service) resolveManagementTarget(
	ctx context.Context,
	userID string,
	driverID string,
) (*models.Driver, error) {
	if userID == "" || driverID == "" {
		return nil, ErrDriverNotFound
	}

	if s.repo == nil {
		return nil, fmt.Errorf("driver repository is not configured")
	}

	driver, err := s.repo.GetByID(ctx, driverID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDriverNotFound
		}

		return nil, fmt.Errorf("get driver for management authority: %w", err)
	}

	if s.userRoles == nil {
		return nil, fmt.Errorf("user role repository is not configured")
	}

	roles, err := s.userRoles.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}

	for _, role := range roles {
		if role == "SYSTEM_ADMIN" {
			return driver, nil
		}
	}

	if s.companyMemberships == nil {
		return nil, fmt.Errorf(
			"company membership repository is not configured",
		)
	}

	member, err := s.companyMemberships.Exists(
		ctx,
		userID,
		driver.CompanyID,
	)
	if err != nil {
		return nil, fmt.Errorf("check company membership: %w", err)
	}

	if !member {
		return nil, ErrDriverNotFound
	}

	return driver, nil
}
