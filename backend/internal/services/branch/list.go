package branch

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) List(
	ctx context.Context,
) ([]*models.Branch, error) {

	branches, err := s.branches.List(ctx)
	if err != nil {
		return nil, err
	}

	return branches, nil
}