package fleet

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) GetByID(
	ctx context.Context,
	userID string,
	id string,
) (*FleetResponse, error) {
	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, err
	}

	var fleet *models.Fleet

	if systemAdmin {
		fleet, err = s.fleets.GetByID(ctx, id)
	} else {
		fleet, err = s.fleets.GetByIDForCompanyMember(
			ctx,
			userID,
			id,
		)
	}

	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrFleetNotFound
		}

		return nil, err
	}

	return fleetResponse(fleet), nil
}

func fleetResponse(fleet *models.Fleet) *FleetResponse {
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
	}
}
