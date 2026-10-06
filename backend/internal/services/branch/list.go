package branch

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) List(
	ctx context.Context,
	userID string,
) ([]*models.Branch, error) {
	if s == nil || s.branches == nil || s.userRoles == nil || userID == "" {
		return nil, fmt.Errorf("branch list access denied")
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve branch list authority: %w", err)
	}

	if systemAdmin {
		return s.branches.List(ctx)
	}

	return s.branches.ListForCompanyMember(ctx, userID)
}
