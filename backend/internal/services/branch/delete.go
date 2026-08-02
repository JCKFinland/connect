package branch

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) Delete(
	ctx context.Context,
	id string,
) error {

	_, err := s.branches.GetByID(ctx, id)
	if err != nil {
		if err == repository.ErrNotFound {
			return ErrBranchNotFound
		}
		return err
	}

	return s.branches.Delete(ctx, id)
}