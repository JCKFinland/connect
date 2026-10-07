package driver

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List returns drivers visible through the authenticated caller's tenant boundary.
func (s *Service) List(
	ctx context.Context,
	userID string,
) ([]models.Driver, error) {
	if s == nil ||
		s.repo == nil ||
		s.userRoles == nil ||
		userID == "" {
		return nil, fmt.Errorf("driver list access denied")
	}

	roles, err := s.userRoles.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve driver list authority: %w", err)
	}

	for _, role := range roles {
		if role == "SYSTEM_ADMIN" {
			return s.repo.List(ctx)
		}
	}

	return s.repo.ListForCompanyMember(ctx, userID)
}
