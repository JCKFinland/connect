package vehicle

import (
	"context"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type vehicleUserRoleRepositoryStub struct {
	roles []string
	err   error
}

func (r *vehicleUserRoleRepositoryStub) AssignRole(
	ctx context.Context,
	userID string,
	roleID string,
) error {
	return nil
}

func (r *vehicleUserRoleRepositoryStub) RemoveRole(
	ctx context.Context,
	userID string,
	roleID string,
) error {
	return nil
}

func (r *vehicleUserRoleRepositoryStub) UserHasRole(
	ctx context.Context,
	userID string,
	roleID string,
) (bool, error) {
	return false, nil
}

func (r *vehicleUserRoleRepositoryStub) GetUserRoles(
	ctx context.Context,
	userID string,
) ([]string, error) {
	return r.roles, r.err
}

type vehicleCompanyMembershipRepositoryStub struct {
	memberships map[string]map[string]bool
	err         error
}

func (r *vehicleCompanyMembershipRepositoryStub) Create(
	ctx context.Context,
	membership *models.CompanyMembership,
) error {
	return nil
}

func (r *vehicleCompanyMembershipRepositoryStub) Exists(
	ctx context.Context,
	userID string,
	companyID string,
) (bool, error) {
	if r.err != nil {
		return false, r.err
	}

	companies := r.memberships[userID]
	return companies != nil && companies[companyID], nil
}

func (r *vehicleCompanyMembershipRepositoryStub) ListByUserID(
	ctx context.Context,
	userID string,
) ([]*models.CompanyMembership, error) {
	return nil, nil
}

func TestCreateDerivesTenantFromFleetAndCreatesActiveVehicle(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{}
	fleetRepo := &vehicleFleetRepositoryStub{
		fleet: &models.Fleet{
			BaseModel: models.BaseModel{ID: "fleet-1"},
			CompanyID: "company-1",
			BranchID:  "branch-1",
			IsActive:  true,
		},
	}

	service := NewService(Dependencies{
		Vehicles:  vehicleRepo,
		Fleets:    fleetRepo,
		UserRoles: &vehicleUserRoleRepositoryStub{roles: []string{"COMPANY_ADMIN"}},
		CompanyMemberships: &vehicleCompanyMembershipRepositoryStub{
			memberships: map[string]map[string]bool{
				"user-1": {"company-1": true},
			},
		},
	})

	result, err := service.Create(
		context.Background(),
		"user-1",
		CreateVehicleRequest{
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
	if err != nil {
		t.Fatalf("create vehicle: %v", err)
	}

	if vehicleRepo.created == nil {
		t.Fatal("expected vehicle to be created")
	}

	if vehicleRepo.created.CompanyID != "company-1" {
		t.Fatalf(
			"expected company derived from fleet, got %q",
			vehicleRepo.created.CompanyID,
		)
	}
	if vehicleRepo.created.BranchID != "branch-1" {
		t.Fatalf(
			"expected branch derived from fleet, got %q",
			vehicleRepo.created.BranchID,
		)
	}
	if vehicleRepo.created.FleetID != "fleet-1" {
		t.Fatalf(
			"expected fleet fleet-1, got %q",
			vehicleRepo.created.FleetID,
		)
	}
	if !vehicleRepo.created.IsActive {
		t.Fatal("expected service-owned initial active state")
	}
	if result.CompanyID != "company-1" ||
		result.BranchID != "branch-1" ||
		result.FleetID != "fleet-1" {
		t.Fatalf("unexpected response tenant: %#v", result)
	}
}

func TestCreateRejectsCrossTenantMembership(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{}

	service := NewService(Dependencies{
		Vehicles: vehicleRepo,
		Fleets: &vehicleFleetRepositoryStub{
			fleet: &models.Fleet{
				BaseModel: models.BaseModel{ID: "fleet-2"},
				CompanyID: "company-2",
				BranchID:  "branch-2",
				IsActive:  true,
			},
		},
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
		CompanyMemberships: &vehicleCompanyMembershipRepositoryStub{
			memberships: map[string]map[string]bool{
				"user-1": {"company-1": true},
			},
		},
	})

	_, err := service.Create(
		context.Background(),
		"user-1",
		CreateVehicleRequest{
			FleetID:            "fleet-2",
			RegistrationNumber: "ABC-123",
			Make:               "Toyota",
			Model:              "Corolla",
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
		},
	)

	if err != ErrVehicleCreationAccessDenied {
		t.Fatalf(
			"expected ErrVehicleCreationAccessDenied, got %v",
			err,
		)
	}
	if vehicleRepo.created != nil {
		t.Fatal("cross-tenant vehicle must not be created")
	}
}

func TestCreateAllowsSystemAdminWithoutCompanyMembership(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{}

	service := NewService(Dependencies{
		Vehicles: vehicleRepo,
		Fleets: &vehicleFleetRepositoryStub{
			fleet: &models.Fleet{
				BaseModel: models.BaseModel{ID: "fleet-1"},
				CompanyID: "company-1",
				BranchID:  "branch-1",
				IsActive:  true,
			},
		},
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
		CompanyMemberships: &vehicleCompanyMembershipRepositoryStub{},
	})

	_, err := service.Create(
		context.Background(),
		"system-admin",
		CreateVehicleRequest{
			FleetID:            "fleet-1",
			RegistrationNumber: "ABC-123",
			Make:               "Toyota",
			Model:              "Corolla",
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
		},
	)
	if err != nil {
		t.Fatalf("system admin create vehicle: %v", err)
	}
	if vehicleRepo.created == nil {
		t.Fatal("expected system admin vehicle creation")
	}
}

func TestCreateRejectsInactiveFleet(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{}

	service := NewService(Dependencies{
		Vehicles: vehicleRepo,
		Fleets: &vehicleFleetRepositoryStub{
			fleet: &models.Fleet{
				BaseModel: models.BaseModel{ID: "fleet-1"},
				CompanyID: "company-1",
				BranchID:  "branch-1",
				IsActive:  false,
			},
		},
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
		CompanyMemberships: &vehicleCompanyMembershipRepositoryStub{},
	})

	_, err := service.Create(
		context.Background(),
		"system-admin",
		CreateVehicleRequest{
			FleetID:            "fleet-1",
			RegistrationNumber: "ABC-123",
			Make:               "Toyota",
			Model:              "Corolla",
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
		},
	)

	if err != ErrInvalidFleet {
		t.Fatalf("expected ErrInvalidFleet, got %v", err)
	}
	if vehicleRepo.created != nil {
		t.Fatal("vehicle must not be created in inactive fleet")
	}
}

var _ repository.UserRoleRepository = (*vehicleUserRoleRepositoryStub)(nil)
var _ repository.CompanyMembershipRepository = (*vehicleCompanyMembershipRepositoryStub)(nil)
