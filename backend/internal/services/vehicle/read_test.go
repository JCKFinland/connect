package vehicle

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func TestGetByIDUsesMembershipScopedReadForNonSystemAdmin(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{
		vehicle: &models.Vehicle{
			BaseModel: models.BaseModel{ID: "vehicle-1"},
			CompanyID: "company-1",
		},
	}

	service := NewService(Dependencies{
		Vehicles: vehicleRepo,
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
	})

	result, err := service.GetByID(
		context.Background(),
		"user-1",
		"vehicle-1",
	)
	if err != nil {
		t.Fatalf("get vehicle: %v", err)
	}
	if result.ID != "vehicle-1" {
		t.Fatalf("expected vehicle-1, got %q", result.ID)
	}
	if vehicleRepo.memberGetCalls != 1 || vehicleRepo.globalGetCalls != 0 {
		t.Fatalf(
			"expected only membership-scoped get, member=%d global=%d",
			vehicleRepo.memberGetCalls,
			vehicleRepo.globalGetCalls,
		)
	}
}

func TestGetByIDUsesGlobalReadForSystemAdmin(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{
		vehicle: &models.Vehicle{
			BaseModel: models.BaseModel{ID: "vehicle-1"},
			CompanyID: "company-1",
		},
	}

	service := NewService(Dependencies{
		Vehicles: vehicleRepo,
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	_, err := service.GetByID(
		context.Background(),
		"system-admin",
		"vehicle-1",
	)
	if err != nil {
		t.Fatalf("get vehicle as system admin: %v", err)
	}
	if vehicleRepo.globalGetCalls != 1 || vehicleRepo.memberGetCalls != 0 {
		t.Fatalf(
			"expected only global get, global=%d member=%d",
			vehicleRepo.globalGetCalls,
			vehicleRepo.memberGetCalls,
		)
	}
}

func TestGetByIDPreservesNotFoundForCrossTenantRead(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{
		getErr: repository.ErrNotFound,
	}

	service := NewService(Dependencies{
		Vehicles: vehicleRepo,
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
	})

	_, err := service.GetByID(
		context.Background(),
		"user-1",
		"other-company-vehicle",
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if vehicleRepo.globalGetCalls != 0 {
		t.Fatal("cross-tenant read must not fall back to global get")
	}
}

func TestListUsesMembershipScopedReadForNonSystemAdmin(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{
		vehicles: []models.Vehicle{
			{
				BaseModel: models.BaseModel{ID: "vehicle-1"},
				CompanyID: "company-1",
			},
		},
	}

	service := NewService(Dependencies{
		Vehicles: vehicleRepo,
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"DISPATCHER"},
		},
	})

	vehicles, err := service.List(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("list vehicles: %v", err)
	}
	if len(vehicles) != 1 || vehicles[0].ID != "vehicle-1" {
		t.Fatalf("unexpected vehicles: %#v", vehicles)
	}
	if vehicleRepo.memberListCalls != 1 || vehicleRepo.globalListCalls != 0 {
		t.Fatalf(
			"expected only membership-scoped list, member=%d global=%d",
			vehicleRepo.memberListCalls,
			vehicleRepo.globalListCalls,
		)
	}
}

func TestListUsesGlobalReadForSystemAdmin(t *testing.T) {
	vehicleRepo := &vehicleRepositoryStub{
		vehicles: []models.Vehicle{
			{BaseModel: models.BaseModel{ID: "vehicle-1"}},
			{BaseModel: models.BaseModel{ID: "vehicle-2"}},
		},
	}

	service := NewService(Dependencies{
		Vehicles: vehicleRepo,
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	vehicles, err := service.List(context.Background(), "system-admin")
	if err != nil {
		t.Fatalf("list vehicles as system admin: %v", err)
	}
	if len(vehicles) != 2 {
		t.Fatalf("expected 2 vehicles, got %d", len(vehicles))
	}
	if vehicleRepo.globalListCalls != 1 || vehicleRepo.memberListCalls != 0 {
		t.Fatalf(
			"expected only global list, global=%d member=%d",
			vehicleRepo.globalListCalls,
			vehicleRepo.memberListCalls,
		)
	}
}
