package vehicle

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
)

type vehicleRepositoryStub struct {
	created   *models.Vehicle
	createErr error
}

func (r *vehicleRepositoryStub) Create(
	ctx context.Context,
	vehicle *models.Vehicle,
) error {
	if r.createErr != nil {
		return r.createErr
	}

	vehicle.ID = "vehicle-1"
	r.created = vehicle

	return nil
}

func (r *vehicleRepositoryStub) GetByID(
	ctx context.Context,
	id string,
) (*models.Vehicle, error) {
	return nil, repository.ErrNotFound
}

func (r *vehicleRepositoryStub) List(
	ctx context.Context,
) ([]models.Vehicle, error) {
	return nil, nil
}

func (r *vehicleRepositoryStub) Update(
	ctx context.Context,
	vehicle *models.Vehicle,
) error {
	return nil
}

func (r *vehicleRepositoryStub) Delete(
	ctx context.Context,
	id string,
) error {
	return nil
}

type vehicleDriverRepositoryStub struct {
	driver *models.Driver
	err    error
}

func (r *vehicleDriverRepositoryStub) Create(
	ctx context.Context,
	driver *models.Driver,
) error {
	return nil
}

func (r *vehicleDriverRepositoryStub) GetByID(
	ctx context.Context,
	id string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (r *vehicleDriverRepositoryStub) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.Driver, error) {
	return r.GetByID(ctx, id)
}

func (r *vehicleDriverRepositoryStub) GetByUserID(
	ctx context.Context,
	userID string,
) (*models.Driver, error) {
	if r.err != nil {
		return nil, r.err
	}

	if r.driver == nil {
		return nil, repository.ErrNotFound
	}

	return r.driver, nil
}

func (r *vehicleDriverRepositoryStub) List(
	ctx context.Context,
) ([]models.Driver, error) {
	return nil, nil
}

func (r *vehicleDriverRepositoryStub) Update(
	ctx context.Context,
	driver *models.Driver,
) error {
	return nil
}

func (r *vehicleDriverRepositoryStub) Delete(
	ctx context.Context,
	id string,
) error {
	return nil
}

type vehicleFleetRepositoryStub struct {
	fleet *models.Fleet
	err   error
}

func (r *vehicleFleetRepositoryStub) Create(
	ctx context.Context,
	fleet *models.Fleet,
) error {
	return nil
}

func (r *vehicleFleetRepositoryStub) GetByID(
	ctx context.Context,
	id string,
) (*models.Fleet, error) {
	if r.err != nil {
		return nil, r.err
	}

	if r.fleet == nil {
		return nil, repository.ErrNotFound
	}

	return r.fleet, nil
}

func (r *vehicleFleetRepositoryStub) List(
	ctx context.Context,
) ([]*models.Fleet, error) {
	return nil, nil
}

func (r *vehicleFleetRepositoryStub) Update(
	ctx context.Context,
	fleet *models.Fleet,
) error {
	return nil
}

func (r *vehicleFleetRepositoryStub) Delete(
	ctx context.Context,
	id string,
) error {
	return nil
}

func TestRegisterDerivesDriverTenantAndCreatesVehicle(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{}

	driverRepo := &vehicleDriverRepositoryStub{
		driver: &models.Driver{
			BaseModel: models.BaseModel{
				ID: "driver-1",
			},
			UserID:     "user-1",
			CompanyID:  "company-1",
			BranchID:   "branch-1",
			Status:     "ACTIVE",
			IsVerified: true,
			IsActive:   true,
		},
	}

	fleetRepo := &vehicleFleetRepositoryStub{
		fleet: &models.Fleet{
			BaseModel: models.BaseModel{
				ID: "fleet-1",
			},
			CompanyID: "company-1",
			BranchID:  "branch-1",
			IsActive:  true,
		},
	}

	service := NewService(
		Dependencies{
			Vehicles: vehicleRepo,
			Drivers:  driverRepo,
			Fleets:   fleetRepo,
		},
	)

	result, err := service.Register(
		context.Background(),
		"user-1",
		RegisterDriverVehicleRequest{
			FleetID:            "fleet-1",
			RegistrationNumber: " abc-123 ",
			VIN:                " abcdefghijklmn123 ",
			Make:               "Toyota",
			Model:              "Corolla",
			ModelYear:          2025,
			Color:              "Black",
			VehicleType:        "sedan",
			FuelType:           "hybrid",
			SeatingCapacity:    4,
		},
	)
	if err != nil {
		t.Fatalf("register vehicle: %v", err)
	}

	if vehicleRepo.created == nil {
		t.Fatal("expected vehicle to be created")
	}

	if vehicleRepo.created.CompanyID != "company-1" {
		t.Fatalf(
			"expected company company-1, got %q",
			vehicleRepo.created.CompanyID,
		)
	}

	if vehicleRepo.created.BranchID != "branch-1" {
		t.Fatalf(
			"expected branch branch-1, got %q",
			vehicleRepo.created.BranchID,
		)
	}

	if vehicleRepo.created.FleetID != "fleet-1" {
		t.Fatalf(
			"expected fleet fleet-1, got %q",
			vehicleRepo.created.FleetID,
		)
	}

	if vehicleRepo.created.RegistrationNumber != "ABC-123" {
		t.Fatalf(
			"expected normalized registration ABC-123, got %q",
			vehicleRepo.created.RegistrationNumber,
		)
	}

	if vehicleRepo.created.VIN == nil ||
		*vehicleRepo.created.VIN != "ABCDEFGHIJKLMN123" {
		t.Fatalf("expected normalized VIN, got %#v", vehicleRepo.created.VIN)
	}

	if vehicleRepo.created.VehicleType != "SEDAN" {
		t.Fatalf(
			"expected vehicle type SEDAN, got %q",
			vehicleRepo.created.VehicleType,
		)
	}

	if vehicleRepo.created.FuelType != "HYBRID" {
		t.Fatalf(
			"expected fuel type HYBRID, got %q",
			vehicleRepo.created.FuelType,
		)
	}

	if !vehicleRepo.created.IsActive {
		t.Fatal("expected registered vehicle to be active")
	}

	if result.ID != "vehicle-1" {
		t.Fatalf("expected vehicle-1, got %q", result.ID)
	}
}

func TestRegisterRejectsUnverifiedDriver(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{}

	driverRepo := &vehicleDriverRepositoryStub{
		driver: &models.Driver{
			UserID:     "user-1",
			CompanyID:  "company-1",
			BranchID:   "branch-1",
			Status:     "ACTIVE",
			IsVerified: false,
			IsActive:   true,
		},
	}

	service := NewService(
		Dependencies{
			Vehicles: vehicleRepo,
			Drivers:  driverRepo,
			Fleets:   &vehicleFleetRepositoryStub{},
		},
	)

	_, err := service.Register(
		context.Background(),
		"user-1",
		RegisterDriverVehicleRequest{
			FleetID:            "fleet-1",
			RegistrationNumber: "ABC-123",
			Make:               "Toyota",
			Model:              "Corolla",
			ModelYear:          2025,
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
			SeatingCapacity:    4,
		},
	)

	if !errors.Is(err, ErrDriverNotEligible) {
		t.Fatalf(
			"expected ErrDriverNotEligible, got %v",
			err,
		)
	}

	if vehicleRepo.created != nil {
		t.Fatal("vehicle must not be created for unverified driver")
	}
}

func TestRegisterRejectsFleetOutsideDriverTenant(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{}

	driverRepo := &vehicleDriverRepositoryStub{
		driver: &models.Driver{
			UserID:     "user-1",
			CompanyID:  "company-1",
			BranchID:   "branch-1",
			Status:     "ACTIVE",
			IsVerified: true,
			IsActive:   true,
		},
	}

	fleetRepo := &vehicleFleetRepositoryStub{
		fleet: &models.Fleet{
			BaseModel: models.BaseModel{
				ID: "fleet-2",
			},
			CompanyID: "company-2",
			BranchID:  "branch-2",
			IsActive:  true,
		},
	}

	service := NewService(
		Dependencies{
			Vehicles: vehicleRepo,
			Drivers:  driverRepo,
			Fleets:   fleetRepo,
		},
	)

	_, err := service.Register(
		context.Background(),
		"user-1",
		RegisterDriverVehicleRequest{
			FleetID:            "fleet-2",
			RegistrationNumber: "ABC-123",
			Make:               "Toyota",
			Model:              "Corolla",
			ModelYear:          2025,
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
			SeatingCapacity:    4,
		},
	)

	if !errors.Is(err, ErrInvalidFleet) {
		t.Fatalf(
			"expected ErrInvalidFleet, got %v",
			err,
		)
	}

	if vehicleRepo.created != nil {
		t.Fatal("vehicle must not be created for fleet outside driver tenant")
	}
}

func TestRegisterRejectsInvalidFuelType(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{}

	driverRepo := &vehicleDriverRepositoryStub{
		driver: &models.Driver{
			UserID:     "user-1",
			CompanyID:  "company-1",
			BranchID:   "branch-1",
			Status:     "ACTIVE",
			IsVerified: true,
			IsActive:   true,
		},
	}

	service := NewService(
		Dependencies{
			Vehicles: vehicleRepo,
			Drivers:  driverRepo,
			Fleets:   &vehicleFleetRepositoryStub{},
		},
	)

	_, err := service.Register(
		context.Background(),
		"user-1",
		RegisterDriverVehicleRequest{
			FleetID:            "fleet-1",
			RegistrationNumber: "ABC-123",
			Make:               "Toyota",
			Model:              "Corolla",
			ModelYear:          2025,
			VehicleType:        "SEDAN",
			FuelType:           "HYDROGEN",
			SeatingCapacity:    4,
		},
	)

	if !errors.Is(err, ErrInvalidVehicle) {
		t.Fatalf(
			"expected ErrInvalidVehicle, got %v",
			err,
		)
	}

	if vehicleRepo.created != nil {
		t.Fatal("vehicle must not be created with invalid fuel type")
	}
}

func TestRegisterMapsDuplicateRegistrationNumber(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{
		createErr: &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "idx_vehicles_registration",
		},
	}

	driverRepo := &vehicleDriverRepositoryStub{
		driver: &models.Driver{
			UserID:     "user-1",
			CompanyID:  "company-1",
			BranchID:   "branch-1",
			Status:     "ACTIVE",
			IsVerified: true,
			IsActive:   true,
		},
	}

	fleetRepo := &vehicleFleetRepositoryStub{
		fleet: &models.Fleet{
			BaseModel: models.BaseModel{
				ID: "fleet-1",
			},
			CompanyID: "company-1",
			BranchID:  "branch-1",
			IsActive:  true,
		},
	}

	service := NewService(
		Dependencies{
			Vehicles: vehicleRepo,
			Drivers:  driverRepo,
			Fleets:   fleetRepo,
		},
	)

	_, err := service.Register(
		context.Background(),
		"user-1",
		RegisterDriverVehicleRequest{
			FleetID:            "fleet-1",
			RegistrationNumber: "ABC-123",
			Make:               "Toyota",
			Model:              "Corolla",
			ModelYear:          2025,
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
			SeatingCapacity:    4,
		},
	)

	if !errors.Is(err, ErrDuplicateRegistrationNumber) {
		t.Fatalf(
			"expected ErrDuplicateRegistrationNumber, got %v",
			err,
		)
	}
}

func TestRegisterMapsDuplicateVIN(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{
		createErr: &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "idx_vehicles_vin",
		},
	}

	driverRepo := &vehicleDriverRepositoryStub{
		driver: &models.Driver{
			UserID:     "user-1",
			CompanyID:  "company-1",
			BranchID:   "branch-1",
			Status:     "ACTIVE",
			IsVerified: true,
			IsActive:   true,
		},
	}

	fleetRepo := &vehicleFleetRepositoryStub{
		fleet: &models.Fleet{
			BaseModel: models.BaseModel{
				ID: "fleet-1",
			},
			CompanyID: "company-1",
			BranchID:  "branch-1",
			IsActive:  true,
		},
	}

	service := NewService(
		Dependencies{
			Vehicles: vehicleRepo,
			Drivers:  driverRepo,
			Fleets:   fleetRepo,
		},
	)

	_, err := service.Register(
		context.Background(),
		"user-1",
		RegisterDriverVehicleRequest{
			FleetID:            "fleet-1",
			RegistrationNumber: "ABC-123",
			VIN:                "ABCDEFGHJKLMN1234",
			Make:               "Toyota",
			Model:              "Corolla",
			ModelYear:          2025,
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
			SeatingCapacity:    4,
		},
	)

	if !errors.Is(err, ErrDuplicateVIN) {
		t.Fatalf(
			"expected ErrDuplicateVIN, got %v",
			err,
		)
	}
}

func TestRegisterDoesNotMapUnrelatedUniqueConstraint(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{
		createErr: &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "some_other_unique_constraint",
		},
	}

	driverRepo := &vehicleDriverRepositoryStub{
		driver: &models.Driver{
			UserID:     "user-1",
			CompanyID:  "company-1",
			BranchID:   "branch-1",
			Status:     "ACTIVE",
			IsVerified: true,
			IsActive:   true,
		},
	}

	fleetRepo := &vehicleFleetRepositoryStub{
		fleet: &models.Fleet{
			BaseModel: models.BaseModel{
				ID: "fleet-1",
			},
			CompanyID: "company-1",
			BranchID:  "branch-1",
			IsActive:  true,
		},
	}

	service := NewService(
		Dependencies{
			Vehicles: vehicleRepo,
			Drivers:  driverRepo,
			Fleets:   fleetRepo,
		},
	)

	_, err := service.Register(
		context.Background(),
		"user-1",
		RegisterDriverVehicleRequest{
			FleetID:            "fleet-1",
			RegistrationNumber: "ABC-123",
			Make:               "Toyota",
			Model:              "Corolla",
			ModelYear:          2025,
			VehicleType:        "SEDAN",
			FuelType:           "PETROL",
			SeatingCapacity:    4,
		},
	)

	if err == nil {
		t.Fatal("expected database error")
	}

	if errors.Is(err, ErrDuplicateRegistrationNumber) {
		t.Fatal("unrelated constraint must not map to duplicate registration")
	}

	if errors.Is(err, ErrDuplicateVIN) {
		t.Fatal("unrelated constraint must not map to duplicate VIN")
	}
}
