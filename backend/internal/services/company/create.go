package company

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) Create(
	ctx context.Context,
	req CreateCompanyRequest,
) (*models.Company, error) {

	company := &models.Company{
		Name:          req.Name,
		LegalName:     req.LegalName,
		BusinessID:    req.BusinessID,
		Email:         req.Email,
		Phone:         req.Phone,
		Website:       req.Website,
		CountryCode:   req.CountryCode,
		Timezone:      req.Timezone,
		AddressLine1:  req.AddressLine1,
		AddressLine2:  req.AddressLine2,
		City:          req.City,
		State:         req.State,
		PostalCode:    req.PostalCode,
		LogoURL:       req.LogoURL,
		IsActive:      req.IsActive,
	}

	if err := s.companies.Create(
		ctx,
		company,
	); err != nil {
		return nil, err
	}

	return company, nil
}