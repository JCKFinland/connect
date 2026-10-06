package branch

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Update modifies descriptive branch fields within the authenticated caller's
// tenant authority. Company and activation authority are immutable here.
func (s *Service) Update(
	ctx context.Context,
	userID string,
	id string,
	req UpdateBranchRequest,
) error {
	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return err
	}

	branch := &models.Branch{
		BaseModel: models.BaseModel{
			ID: id,
		},
		Code:         req.Code,
		Name:         req.Name,
		Email:        req.Email,
		Phone:        req.Phone,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		City:         req.City,
		State:        req.State,
		PostalCode:   req.PostalCode,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
	}

	if systemAdmin {
		err = s.branches.UpdateDetails(ctx, branch)
	} else {
		err = s.branches.UpdateDetailsForCompanyMember(
			ctx,
			userID,
			branch,
		)
	}

	if errors.Is(err, repository.ErrNotFound) {
		return ErrBranchNotFound
	}

	return err
}
