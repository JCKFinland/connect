package driver

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Create registers a new driver.
func (s *Service) Create(
	ctx context.Context,
	req CreateDriverRequest,
) (*models.Driver, error) {

	now := time.Now().UTC()

	driver := &models.Driver{
		BaseModel: models.BaseModel{
			ID:        uuid.NewString(),
			CreatedAt: now,
			UpdatedAt: now,
		},

		UserID:      req.UserID,
		CompanyID:   req.CompanyID,
		BranchID:    req.BranchID,
		DriverNumber: req.DriverNumber,

		FirstName: req.FirstName,
		LastName:  req.LastName,

		Phone: req.Phone,
		Email: req.Email,

		TaxiDriverLicenseNumber: req.TaxiDriverLicenseNumber,

		DrivingLicenseNumber: req.DrivingLicenseNumber,
		DrivingLicenseExpiry: req.DrivingLicenseExpiry,

		HireDate: req.HireDate,

		Status: req.Status,

		IsVerified: req.IsVerified,
		IsActive:   req.IsActive,
	}

	// Apply sensible defaults.
	if driver.Status == "" {
		driver.Status = "ACTIVE"
	}

	if !driver.IsActive {
		driver.IsActive = true
	}

	err := s.repo.Create(
		ctx,
		driver,
	)
	if err != nil {
		return nil, err
	}

	return driver, nil
}