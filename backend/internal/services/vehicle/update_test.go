package vehicle

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

func TestUpdateUsesMembershipGuardedMutationForNonSystemAdmin(t *testing.T) {
	repo := &vehicleRepositoryStub{}

	service := NewService(Dependencies{
		Vehicles: repo,
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
	})

	err := service.Update(
		context.Background(),
		"user-1",
		"vehicle-1",
		UpdateVehicleRequest{
			RegistrationNumber: "ABC-123",
			VIN:                " abcdefghijklmn123 ",
			Make:               "Toyota",
			Model:              "Corolla",
			ModelYear:          2025,
			Color:              "Black",
			VehicleType:        "SEDAN",
			FuelType:           "HYBRID",
			SeatingCapacity:    4,
		},
	)
	if err != nil {
		t.Fatalf("update vehicle: %v", err)
	}

	if repo.memberUpdateCalls != 1 {
		t.Fatalf(
			"expected one membership-guarded update, got %d",
			repo.memberUpdateCalls,
		)
	}
	if repo.globalUpdateCalls != 0 {
		t.Fatalf(
			"non-system update used unrestricted mutation %d times",
			repo.globalUpdateCalls,
		)
	}
	if repo.globalGetCalls != 0 || repo.memberGetCalls != 0 {
		t.Fatalf(
			"update must not authorize through read-before-write, global=%d member=%d",
			repo.globalGetCalls,
			repo.memberGetCalls,
		)
	}
	if repo.updateUserID != "user-1" {
		t.Fatalf(
			"expected guarded mutation user user-1, got %q",
			repo.updateUserID,
		)
	}
	if repo.updated == nil {
		t.Fatal("expected descriptive update")
	}

	if repo.updated.CompanyID != "" ||
		repo.updated.BranchID != "" ||
		repo.updated.FleetID != "" ||
		repo.updated.IsActive {
		t.Fatalf(
			"protected authority fields leaked into update: %#v",
			repo.updated,
		)
	}

	if repo.updated.VIN == nil ||
		*repo.updated.VIN != "ABCDEFGHIJKLMN123" {
		t.Fatalf(
			"expected normalized VIN, got %#v",
			repo.updated.VIN,
		)
	}
}

func TestUpdateUsesUnrestrictedMutationOnlyForSystemAdmin(t *testing.T) {
	repo := &vehicleRepositoryStub{}

	service := NewService(Dependencies{
		Vehicles: repo,
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	err := service.Update(
		context.Background(),
		"system-admin",
		"vehicle-1",
		UpdateVehicleRequest{
			Make: "Toyota",
		},
	)
	if err != nil {
		t.Fatalf("system admin update: %v", err)
	}

	if repo.globalUpdateCalls != 1 {
		t.Fatalf(
			"expected one unrestricted system-admin update, got %d",
			repo.globalUpdateCalls,
		)
	}
	if repo.memberUpdateCalls != 0 {
		t.Fatalf(
			"system admin unexpectedly used guarded mutation %d times",
			repo.memberUpdateCalls,
		)
	}
	if repo.globalGetCalls != 0 || repo.memberGetCalls != 0 {
		t.Fatalf(
			"system-admin update unexpectedly performed target read, global=%d member=%d",
			repo.globalGetCalls,
			repo.memberGetCalls,
		)
	}
}

func TestUpdatePreservesNotFoundFromMembershipGuardedMutation(t *testing.T) {
	repo := &vehicleRepositoryStub{
		updateErr: repository.ErrNotFound,
	}

	service := NewService(Dependencies{
		Vehicles: repo,
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
	})

	err := service.Update(
		context.Background(),
		"user-1",
		"other-company-vehicle",
		UpdateVehicleRequest{
			Make: "Toyota",
		},
	)

	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound, got %v",
			err,
		)
	}
	if repo.memberUpdateCalls != 1 {
		t.Fatalf(
			"expected one guarded mutation attempt, got %d",
			repo.memberUpdateCalls,
		)
	}
	if repo.globalUpdateCalls != 0 {
		t.Fatalf(
			"cross-tenant update reached unrestricted mutation %d times",
			repo.globalUpdateCalls,
		)
	}
}
