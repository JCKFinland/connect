package company

import (
	"context"
)

func (s *Service) Update(
	ctx context.Context,
	id string,
	req UpdateCompanyRequest,
) error {

	company, err := s.companies.GetByID(
		ctx,
		id,
	)
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
	company.IsActive = req.IsActive

	return s.companies.Update(
		ctx,
		company,
	)
}