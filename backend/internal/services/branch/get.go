package branch

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*models.Branch, error) {

	branch, err := s.branches.GetByID(
		ctx,
		id,
	)
	if err != nil {

		if err == repository.ErrNotFound {
			return nil, ErrBranchNotFound
		}

		return nil, err
	}

	return branch, nil
}