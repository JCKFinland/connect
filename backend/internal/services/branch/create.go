package branch

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) Create(
	ctx context.Context,
	req CreateBranchRequest,
) (*models.Branch, error) {

	branch := &models.Branch{
		CompanyID: req.CompanyID,
		Code: req.Code,
		Name: req.Name,
		Email: req.Email,
		Phone: req.Phone,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		City: req.City,
		State: req.State,
		PostalCode: req.PostalCode,
		Latitude: req.Latitude,
		Longitude: req.Longitude,
		IsActive: req.IsActive,
	}

	if err := s.branches.Create(
		ctx,
		branch,
	); err != nil {
		return nil, err
	}

	return branch, nil
}