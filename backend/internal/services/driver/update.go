package driver

import (
	"context"
	"time"
)

// Update modifies an existing driver.
func (s *Service) Update(
	ctx context.Context,
	id string,
	req UpdateDriverRequest,
) error {

	driver, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return err
	}

	// Update mutable fields.
	driver.CompanyID = req.CompanyID
	driver.BranchID = req.BranchID
	driver.DriverNumber = req.DriverNumber

	driver.FirstName = req.FirstName
	driver.LastName = req.LastName

	driver.Phone = req.Phone
	driver.Email = req.Email

	driver.TaxiDriverLicenseNumber = req.TaxiDriverLicenseNumber
	driver.DrivingLicenseNumber = req.DrivingLicenseNumber
	driver.DrivingLicenseExpiry = req.DrivingLicenseExpiry

	driver.HireDate = req.HireDate

	driver.Status = req.Status
	driver.IsVerified = req.IsVerified
	driver.IsActive = req.IsActive

	driver.UpdatedAt = time.Now().UTC()

	return s.repo.Update(
		ctx,
		driver,
	)
}