package company

import (
	"context"
	"fmt"
)

func (s *Service) isSystemAdmin(
	ctx context.Context,
	userID string,
) (bool, error) {
	if userID == "" {
		return false, fmt.Errorf("user ID is required")
	}

	if s.userRoles == nil {
		return false, fmt.Errorf("user role repository is not configured")
	}

	roles, err := s.userRoles.GetUserRoles(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("get user roles: %w", err)
	}

	for _, role := range roles {
		if role == "SYSTEM_ADMIN" {
			return true, nil
		}
	}

	return false, nil
}
