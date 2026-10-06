package company

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) List(
	ctx context.Context,
	userID string,
) ([]*models.Company, error) {
	if s == nil ||
		s.companies == nil ||
		s.userRoles == nil ||
		userID == "" {
		return nil, fmt.Errorf("company list access denied")
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve company list authority: %w",
			err,
		)
	}

	if systemAdmin {
		return s.companies.List(ctx)
	}

	return s.companies.ListForCompanyMember(ctx, userID)
}
