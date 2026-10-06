package branch

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) GetByID(
	ctx context.Context,
	userID string,
	id string,
) (*models.Branch, error) {
	if s == nil || s.branches == nil || s.userRoles == nil ||
		userID == "" || id == "" {
		return nil, ErrBranchNotFound
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve branch read authority: %w", err)
	}

	var branch *models.Branch
	if systemAdmin {
		branch, err = s.branches.GetByID(ctx, id)
	} else {
		branch, err = s.branches.GetByIDForCompanyMember(
			ctx,
			userID,
			id,
		)
	}

	if err != nil {
		if err == repository.ErrNotFound {
			return nil, ErrBranchNotFound
		}
		return nil, err
	}

	return branch, nil
}
