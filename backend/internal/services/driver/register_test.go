package driver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func TestRegisterRejectsInvalidAuthenticatedUser(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name string
		user *models.User
	}{
		{
			name: "nil user",
			user: nil,
		},
		{
			name: "empty user ID",
			user: &models.User{},
		},
		{
			name: "blank user ID",
			user: &models.User{
				BaseModel: models.BaseModel{ID: "   "},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Register(
				context.Background(),
				tt.user,
				RegisterDriverRequest{},
			)

			if !errors.Is(err, ErrInvalidDriver) {
				t.Fatalf(
					"expected ErrInvalidDriver, got %v",
					err,
				)
			}
		})
	}
}

func TestRegisterRejectsMissingRequiredFields(t *testing.T) {
	expiry := time.Now().UTC().AddDate(1, 0, 0)

	valid := RegisterDriverRequest{
		CompanyID:               "company-1",
		BranchID:                "branch-1",
		TaxiDriverLicenseNumber: "TAXI-123",
		DrivingLicenseNumber:    "DL-123",
		DrivingLicenseExpiry:    &expiry,
	}

	tests := []struct {
		name   string
		mutate func(*RegisterDriverRequest)
	}{
		{
			name: "company ID",
			mutate: func(req *RegisterDriverRequest) {
				req.CompanyID = "   "
			},
		},
		{
			name: "branch ID",
			mutate: func(req *RegisterDriverRequest) {
				req.BranchID = "   "
			},
		},
		{
			name: "taxi driver license number",
			mutate: func(req *RegisterDriverRequest) {
				req.TaxiDriverLicenseNumber = "   "
			},
		},
		{
			name: "driving license number",
			mutate: func(req *RegisterDriverRequest) {
				req.DrivingLicenseNumber = "   "
			},
		},
		{
			name: "driving license expiry",
			mutate: func(req *RegisterDriverRequest) {
				req.DrivingLicenseExpiry = nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			tt.mutate(&req)

			service := &Service{}

			_, err := service.Register(
				context.Background(),
				&models.User{
					BaseModel: models.BaseModel{ID: "user-1"},
				},
				req,
			)

			if !errors.Is(err, ErrInvalidDriver) {
				t.Fatalf(
					"expected ErrInvalidDriver, got %v",
					err,
				)
			}
		})
	}
}

func TestRegisterRejectsExpiredDrivingLicense(t *testing.T) {
	expiry := time.Now().UTC().Add(-24 * time.Hour)

	service := &Service{}

	_, err := service.Register(
		context.Background(),
		&models.User{
			BaseModel: models.BaseModel{ID: "user-1"},
		},
		RegisterDriverRequest{
			CompanyID:               "company-1",
			BranchID:                "branch-1",
			TaxiDriverLicenseNumber: "TAXI-123",
			DrivingLicenseNumber:    "DL-123",
			DrivingLicenseExpiry:    &expiry,
		},
	)

	if !errors.Is(err, ErrInvalidDriver) {
		t.Fatalf(
			"expected ErrInvalidDriver, got %v",
			err,
		)
	}
}

func TestRegisterAcceptsDrivingLicenseExpiringTodayValidation(t *testing.T) {
	now := time.Now().UTC()
	expiry := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		0,
		0,
		0,
		0,
		time.UTC,
	)

	_, err := (&Service{}).Register(
		context.Background(),
		&models.User{
			BaseModel: models.BaseModel{ID: "user-1"},
		},
		RegisterDriverRequest{
			CompanyID:               "company-1",
			BranchID:                "branch-1",
			TaxiDriverLicenseNumber: "TAXI-123",
			DrivingLicenseNumber:    "DL-123",
			DrivingLicenseExpiry:    &expiry,
		},
	)

	if err == nil {
		t.Fatal("expected missing database dependency after successful validation")
	}

	if errors.Is(err, ErrInvalidDriver) {
		t.Fatalf(
			"expected license expiring today to pass validation, got %v",
			err,
		)
	}
}
