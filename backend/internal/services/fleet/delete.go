package fleet

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) Delete(
	ctx context.Context,
	id string,
) error {

	_, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {

		if err == repository.ErrNotFound {
			return ErrFleetNotFound
		}

		return err
	}

	if err := s.repo.Delete(
		ctx,
		id,
	); err != nil {

		if err == repository.ErrNotFound {
			return ErrFleetNotFound
		}

		return err
	}

	return nil
}