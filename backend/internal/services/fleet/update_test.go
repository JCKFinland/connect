package fleet

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

func TestUpdateUsesMembershipGuardedMutationForNonSystemAdmin(t *testing.T) {
	repo := &fleetReadRepositoryStub{}

	service := NewService(Dependencies{
		Fleets: repo,
		UserRoles: &fleetReadUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
	})

	err := service.Update(
		context.Background(),
		"user-1",
		"fleet-1",
		UpdateFleetRequest{
			Code:        "FLEET-NEW",
			Name:        "Updated Fleet",
			Description: "Updated description",
		},
	)
	if err != nil {
		t.Fatalf("update fleet: %v", err)
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
	if repo.memberUserID != "user-1" {
		t.Fatalf(
			"expected guarded update for user-1, got %q",
			repo.memberUserID,
		)
	}
	if repo.updated == nil {
		t.Fatal("expected fleet update payload")
	}
	if repo.updated.ID != "fleet-1" ||
		repo.updated.Code != "FLEET-NEW" ||
		repo.updated.Name != "Updated Fleet" ||
		repo.updated.Description != "Updated description" {
		t.Fatalf("unexpected update payload: %#v", repo.updated)
	}

	// Authority/lifecycle fields must not enter this mutation.
	if repo.updated.CompanyID != "" ||
		repo.updated.BranchID != "" ||
		repo.updated.IsActive {
		t.Fatalf(
			"update payload unexpectedly contains authority fields: %#v",
			repo.updated,
		)
	}
}

func TestUpdateUsesUnrestrictedMutationOnlyForSystemAdmin(t *testing.T) {
	repo := &fleetReadRepositoryStub{}

	service := NewService(Dependencies{
		Fleets: repo,
		UserRoles: &fleetReadUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	err := service.Update(
		context.Background(),
		"system-admin",
		"fleet-1",
		UpdateFleetRequest{
			Code: "FLEET-NEW",
			Name: "Updated Fleet",
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

func TestUpdateMapsMembershipGuardedNotFoundToFleetNotFound(t *testing.T) {
	repo := &fleetReadRepositoryStub{
		updateErr: repository.ErrNotFound,
	}

	service := NewService(Dependencies{
		Fleets: repo,
		UserRoles: &fleetReadUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
	})

	err := service.Update(
		context.Background(),
		"user-1",
		"other-company-fleet",
		UpdateFleetRequest{
			Code: "FLEET-NEW",
			Name: "Updated Fleet",
		},
	)

	if !errors.Is(err, ErrFleetNotFound) {
		t.Fatalf("expected ErrFleetNotFound, got %v", err)
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
	if repo.globalGetCalls != 0 || repo.memberGetCalls != 0 {
		t.Fatalf(
			"cross-tenant update performed read-before-write, global=%d member=%d",
			repo.globalGetCalls,
			repo.memberGetCalls,
		)
	}
}

func TestUpdateMapsSystemAdminNotFoundToFleetNotFound(t *testing.T) {
	repo := &fleetReadRepositoryStub{
		updateErr: repository.ErrNotFound,
	}

	service := NewService(Dependencies{
		Fleets: repo,
		UserRoles: &fleetReadUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	err := service.Update(
		context.Background(),
		"system-admin",
		"missing-fleet",
		UpdateFleetRequest{
			Code: "FLEET-NEW",
			Name: "Updated Fleet",
		},
	)

	if !errors.Is(err, ErrFleetNotFound) {
		t.Fatalf("expected ErrFleetNotFound, got %v", err)
	}
	if repo.globalUpdateCalls != 1 {
		t.Fatalf(
			"expected one unrestricted mutation attempt, got %d",
			repo.globalUpdateCalls,
		)
	}
	if repo.memberUpdateCalls != 0 {
		t.Fatalf(
			"missing system-admin target reached guarded mutation %d times",
			repo.memberUpdateCalls,
		)
	}
}
