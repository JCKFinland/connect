package fleet

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) Update(
	ctx context.Context,
	id string,
	req UpdateFleetRequest,
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

	fleet := &models.Fleet{
		BaseModel: models.BaseModel{
			ID: id,
		},
		CompanyID:  req.CompanyID,
		BranchID:   req.BranchID,
		Code:       req.Code,
		Name:       req.Name,
		Description: req.Description,
		IsActive:   req.IsActive,
	}

	if err := s.repo.Update(
		ctx,
		fleet,
	); err != nil {

		if err == repository.ErrNotFound {
			return ErrFleetNotFound
		}

		return err
	}

	return nil
}