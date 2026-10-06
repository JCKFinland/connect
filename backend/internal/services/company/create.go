package company

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (s *Service) Create(
	ctx context.Context,
	userID string,
	req CreateCompanyRequest,
) (*models.Company, error) {
	if s == nil ||
		s.companies == nil ||
		s.userRoles == nil ||
		userID == "" {
		return nil, ErrCompanyCreationAccessDenied
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve company creation authority: %w",
			err,
		)
	}
	if !systemAdmin {
		return nil, ErrCompanyCreationAccessDenied
	}

	company := &models.Company{
		Name:         req.Name,
		LegalName:    req.LegalName,
		BusinessID:   req.BusinessID,
		Email:        req.Email,
		Phone:        req.Phone,
		Website:      req.Website,
		CountryCode:  req.CountryCode,
		Timezone:     req.Timezone,
		AddressLine1: req.AddressLine1,
		AddressLine2: req.AddressLine2,
		City:         req.City,
		State:        req.State,
		PostalCode:   req.PostalCode,
		LogoURL:      req.LogoURL,
		IsActive:     true,
	}

	if err := s.companies.Create(ctx, company); err != nil {
		return nil, err
	}

	return company, nil
}
