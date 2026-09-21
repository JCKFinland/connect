package driver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Register creates an unverified driver profile for the authenticated user.
func (s *Service) Register(
	ctx context.Context,
	user *models.User,
	req RegisterDriverRequest,
) (*models.Driver, error) {
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return nil, ErrInvalidDriver
	}

	if s.repo == nil || s.branches == nil {
		return nil, fmt.Errorf("driver registration dependencies are not configured")
	}

	// One active driver profile per platform user.
	_, err := s.repo.GetByUserID(ctx, user.ID)
	switch {
	case err == nil:
		return nil, ErrDriverAlreadyExists
	case !errors.Is(err, repository.ErrNotFound):
		return nil, err
	}

	req.CompanyID = strings.TrimSpace(req.CompanyID)
	req.BranchID = strings.TrimSpace(req.BranchID)
	req.TaxiDriverLicenseNumber = strings.TrimSpace(
		req.TaxiDriverLicenseNumber,
	)
	req.DrivingLicenseNumber = strings.TrimSpace(
		req.DrivingLicenseNumber,
	)

	if req.CompanyID == "" ||
		req.BranchID == "" ||
		req.TaxiDriverLicenseNumber == "" ||
		req.DrivingLicenseNumber == "" ||
		req.DrivingLicenseExpiry == nil {
		return nil, ErrInvalidDriver
	}

	if !req.DrivingLicenseExpiry.After(time.Now().UTC()) {
		return nil, ErrInvalidDriver
	}

	branch, err := s.branches.GetByID(ctx, req.BranchID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidDriver
		}

		return nil, err
	}

	if !branch.IsActive || branch.CompanyID != req.CompanyID {
		return nil, ErrInvalidDriver
	}

	now := time.Now().UTC()
	driverID := uuid.NewString()

	driver := &models.Driver{
		BaseModel: models.BaseModel{
			ID:        driverID,
			CreatedAt: now,
			UpdatedAt: now,
		},

		UserID:    user.ID,
		CompanyID: req.CompanyID,
		BranchID:  req.BranchID,

		DriverNumber: "DRV-" + strings.ToUpper(
			strings.ReplaceAll(driverID[:12], "-", ""),
		),

		FirstName: strings.TrimSpace(user.FirstName),
		LastName:  strings.TrimSpace(user.LastName),
		Phone:     strings.TrimSpace(user.Phone),
		Email:     strings.TrimSpace(user.Email),

		TaxiDriverLicenseNumber: req.TaxiDriverLicenseNumber,
		DrivingLicenseNumber:    req.DrivingLicenseNumber,
		DrivingLicenseExpiry:    req.DrivingLicenseExpiry,

		Status:     "PENDING_VERIFICATION",
		IsVerified: false,
		IsActive:   true,
	}

	if err := s.repo.Create(ctx, driver); err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "idx_drivers_unique_active_user" {
			return nil, ErrDriverAlreadyExists
		}

		return nil, fmt.Errorf("create driver registration: %w", err)
	}

	return driver, nil
}
