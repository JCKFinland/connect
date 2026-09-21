package driver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type driverRepositoryStub struct {
	existing  *models.Driver
	created   *models.Driver
	getErr    error
	createErr error
}

func (r *driverRepositoryStub) Create(
	ctx context.Context,
	driver *models.Driver,
) error {
	if r.createErr != nil {
		return r.createErr
	}

	r.created = driver
	return nil
}

func (r *driverRepositoryStub) GetByID(
	ctx context.Context,
	id string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (r *driverRepositoryStub) GetByUserID(
	ctx context.Context,
	userID string,
) (*models.Driver, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}

	if r.existing == nil {
		return nil, repository.ErrNotFound
	}

	return r.existing, nil
}

func (r *driverRepositoryStub) List(
	ctx context.Context,
) ([]models.Driver, error) {
	return nil, nil
}

func (r *driverRepositoryStub) Update(
	ctx context.Context,
	driver *models.Driver,
) error {
	return nil
}

func (r *driverRepositoryStub) Delete(
	ctx context.Context,
	id string,
) error {
	return nil
}

type branchRepositoryStub struct {
	branch *models.Branch
	err    error
}

func (r *branchRepositoryStub) Create(
	ctx context.Context,
	branch *models.Branch,
) error {
	return nil
}

func (r *branchRepositoryStub) Update(
	ctx context.Context,
	branch *models.Branch,
) error {
	return nil
}

func (r *branchRepositoryStub) GetByID(
	ctx context.Context,
	id string,
) (*models.Branch, error) {
	if r.err != nil {
		return nil, r.err
	}

	if r.branch == nil {
		return nil, repository.ErrNotFound
	}

	return r.branch, nil
}

func (r *branchRepositoryStub) List(
	ctx context.Context,
) ([]*models.Branch, error) {
	return nil, nil
}

func (r *branchRepositoryStub) Delete(
	ctx context.Context,
	id string,
) error {
	return nil
}

func TestRegisterDerivesIdentityAndCreatesUnverifiedDriver(t *testing.T) {
	expiry := time.Now().UTC().AddDate(1, 0, 0)

	driverRepo := &driverRepositoryStub{}
	branchRepo := &branchRepositoryStub{
		branch: &models.Branch{
			CompanyID: "company-1",
			IsActive:  true,
		},
	}

	service := NewService(
		Dependencies{
			Drivers:  driverRepo,
			Branches: branchRepo,
		},
	)

	user := &models.User{
		BaseModel: models.BaseModel{
			ID: "user-1",
		},
		Email:     "driver@example.com",
		FirstName: "John",
		LastName:  "Driver",
		Phone:     "+358401234567",
	}

	driver, err := service.Register(
		context.Background(),
		user,
		RegisterDriverRequest{
			CompanyID:               "company-1",
			BranchID:                "branch-1",
			TaxiDriverLicenseNumber: "TAXI-123",
			DrivingLicenseNumber:    "DL-123",
			DrivingLicenseExpiry:    &expiry,
		},
	)
	if err != nil {
		t.Fatalf("register driver: %v", err)
	}

	if driverRepo.created == nil {
		t.Fatal("expected driver to be created")
	}

	if driver.UserID != user.ID {
		t.Fatalf("expected user ID %q, got %q", user.ID, driver.UserID)
	}

	if driver.FirstName != user.FirstName ||
		driver.LastName != user.LastName ||
		driver.Email != user.Email ||
		driver.Phone != user.Phone {
		t.Fatal("expected driver identity to be derived from authenticated user")
	}

	if driver.DriverNumber == "" {
		t.Fatal("expected server-generated driver number")
	}

	if driver.IsVerified {
		t.Fatal("new self-registered driver must not be verified")
	}

	if !driver.IsActive {
		t.Fatal("new self-registered driver must be active")
	}

	if driver.Status != "PENDING_VERIFICATION" {
		t.Fatalf(
			"expected status PENDING_VERIFICATION, got %q",
			driver.Status,
		)
	}
}

func TestRegisterRejectsExistingDriver(t *testing.T) {
	driverRepo := &driverRepositoryStub{
		existing: &models.Driver{
			UserID: "user-1",
		},
	}

	service := NewService(
		Dependencies{
			Drivers:  driverRepo,
			Branches: &branchRepositoryStub{},
		},
	)

	_, err := service.Register(
		context.Background(),
		&models.User{
			BaseModel: models.BaseModel{ID: "user-1"},
		},
		RegisterDriverRequest{},
	)

	if !errors.Is(err, ErrDriverAlreadyExists) {
		t.Fatalf(
			"expected ErrDriverAlreadyExists, got %v",
			err,
		)
	}
}

func TestRegisterRejectsBranchFromDifferentCompany(t *testing.T) {
	expiry := time.Now().UTC().AddDate(1, 0, 0)

	service := NewService(
		Dependencies{
			Drivers: &driverRepositoryStub{},
			Branches: &branchRepositoryStub{
				branch: &models.Branch{
					CompanyID: "company-2",
					IsActive:  true,
				},
			},
		},
	)

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

func TestRegisterRejectsInactiveBranch(t *testing.T) {
	expiry := time.Now().UTC().AddDate(1, 0, 0)

	service := NewService(
		Dependencies{
			Drivers: &driverRepositoryStub{},
			Branches: &branchRepositoryStub{
				branch: &models.Branch{
					CompanyID: "company-1",
					IsActive:  false,
				},
			},
		},
	)

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

func TestRegisterMapsDuplicateUserConstraintToAlreadyExists(t *testing.T) {
	expiry := time.Now().UTC().Add(24 * time.Hour)

	driverRepo := &driverRepositoryStub{
		createErr: &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "idx_drivers_unique_active_user",
		},
	}

	branchRepo := &branchRepositoryStub{
		branch: &models.Branch{
			CompanyID: "company-1",
			IsActive:  true,
		},
	}

	service := NewService(Dependencies{
		Drivers:  driverRepo,
		Branches: branchRepo,
	})

	user := &models.User{}
	user.ID = "user-1"
	user.Email = "driver@example.com"
	user.FirstName = "John"
	user.LastName = "Driver"

	_, err := service.Register(
		context.Background(),
		user,
		RegisterDriverRequest{
			CompanyID:               "company-1",
			BranchID:                "branch-1",
			TaxiDriverLicenseNumber: "TAXI-123",
			DrivingLicenseNumber:    "DL-123",
			DrivingLicenseExpiry:    &expiry,
		},
	)

	if !errors.Is(err, ErrDriverAlreadyExists) {
		t.Fatalf(
			"expected ErrDriverAlreadyExists, got %v",
			err,
		)
	}
}

func TestRegisterDoesNotMapOtherUniqueConstraintsToAlreadyExists(t *testing.T) {
	expiry := time.Now().UTC().Add(24 * time.Hour)

	driverRepo := &driverRepositoryStub{
		createErr: &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "idx_drivers_taxi_license",
		},
	}

	branchRepo := &branchRepositoryStub{
		branch: &models.Branch{
			CompanyID: "company-1",
			IsActive:  true,
		},
	}

	service := NewService(Dependencies{
		Drivers:  driverRepo,
		Branches: branchRepo,
	})

	user := &models.User{}
	user.ID = "user-1"

	_, err := service.Register(
		context.Background(),
		user,
		RegisterDriverRequest{
			CompanyID:               "company-1",
			BranchID:                "branch-1",
			TaxiDriverLicenseNumber: "TAXI-123",
			DrivingLicenseNumber:    "DL-123",
			DrivingLicenseExpiry:    &expiry,
		},
	)

	if err == nil {
		t.Fatal("expected registration error")
	}

	if errors.Is(err, ErrDriverAlreadyExists) {
		t.Fatalf(
			"expected non-user unique constraint to remain distinct, got %v",
			err,
		)
	}
}

func TestRegisterRejectsExpiredDrivingLicense(t *testing.T) {
	expiry := time.Now().UTC().Add(-24 * time.Hour)

	driverRepo := &driverRepositoryStub{}

	branchRepo := &branchRepositoryStub{
		branch: &models.Branch{
			CompanyID: "company-1",
			IsActive:  true,
		},
	}

	service := NewService(Dependencies{
		Drivers:  driverRepo,
		Branches: branchRepo,
	})

	user := &models.User{}
	user.ID = "user-1"

	_, err := service.Register(
		context.Background(),
		user,
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

	if driverRepo.created != nil {
		t.Fatal("expected expired license registration not to be persisted")
	}
}
