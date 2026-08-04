package fleet

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) Create(
	ctx context.Context,
	req CreateFleetRequest,
) (*FleetResponse, error) {

	fleet := &models.Fleet{
		CompanyID:  req.CompanyID,
		BranchID:   req.BranchID,
		Code:       req.Code,
		Name:       req.Name,
		Description: req.Description,
		IsActive:   req.IsActive,
	}

	if err := s.repo.Create(
		ctx,
		fleet,
	); err != nil {
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