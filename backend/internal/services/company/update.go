package company

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func (s *Service) Update(
	ctx context.Context,
	userID string,
	id string,
	req UpdateCompanyRequest,
) error {
	if s == nil ||
		s.companies == nil ||
		s.userRoles == nil ||
		userID == "" ||
		id == "" {
		return ErrCompanyNotFound
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return fmt.Errorf(
			"resolve company update authority: %w",
			err,
		)
	}

	var company *models.Company

	if systemAdmin {
		company, err = s.companies.GetByID(ctx, id)
	} else {
		company, err = s.companies.GetByIDForCompanyMember(
			ctx,
			userID,
			id,
		)
	}

	if errors.Is(err, repository.ErrNotFound) {
		return ErrCompanyNotFound
	}
	if err != nil {
		return err
	}

	company.Name = req.Name
	company.LegalName = req.LegalName
	company.BusinessID = req.BusinessID
	company.Email = req.Email
	company.Phone = req.Phone
	company.Website = req.Website
	company.CountryCode = req.CountryCode
	company.Timezone = req.Timezone
	company.AddressLine1 = req.AddressLine1
	company.AddressLine2 = req.AddressLine2
	company.City = req.City
	company.State = req.State
	company.PostalCode = req.PostalCode
	company.LogoURL = req.LogoURL

	if systemAdmin {
		err = s.companies.UpdateDetails(ctx, company)
	} else {
		err = s.companies.UpdateDetailsForCompanyMember(
			ctx,
			userID,
			company,
		)
	}

	if errors.Is(err, repository.ErrNotFound) {
		return ErrCompanyNotFound
	}

	return err
}
