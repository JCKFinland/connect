package fleet

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*FleetResponse, error) {

	fleet, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {

		if err == repository.ErrNotFound {
			return nil, ErrFleetNotFound
		}

		return nil, err
	}

	return &FleetResponse{
		ID:          fleet.ID,
		CreatedAt:   fleet.CreatedAt,
		UpdatedAt:   fleet.UpdatedAt,
		CompanyID:   fleet.CompanyID,
		BranchID:    fleet.BranchID,
		Code:        fleet.Code,
		Name:        fleet.Name,
		Description: fleet.Description,
		IsActive:    fleet.IsActive,
	}, nil
}