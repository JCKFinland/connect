package fleet

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type fleetReadRepositoryStub struct {
	fleet  *models.Fleet
	fleets []*models.Fleet

	globalGetCalls  int
	memberGetCalls  int
	globalListCalls int
	memberListCalls int

	memberUserID string
	err          error
}

func (r *fleetReadRepositoryStub) Create(
	context.Context,
	*models.Fleet,
) error {
	return nil
}

func (r *fleetReadRepositoryStub) GetByID(
	context.Context,
	string,
) (*models.Fleet, error) {
	r.globalGetCalls++
	if r.err != nil {
		return nil, r.err
	}
	if r.fleet == nil {
		return nil, repository.ErrNotFound
	}
	return r.fleet, nil
}

func (r *fleetReadRepositoryStub) GetByIDForCompanyMember(
	_ context.Context,
	userID string,
	_ string,
) (*models.Fleet, error) {
	r.memberGetCalls++
	r.memberUserID = userID

	if r.err != nil {
		return nil, r.err
	}
	if r.fleet == nil {
		return nil, repository.ErrNotFound
	}
	return r.fleet, nil
}

func (r *fleetReadRepositoryStub) List(
	context.Context,
) ([]*models.Fleet, error) {
	r.globalListCalls++
	if r.err != nil {
		return nil, r.err
	}
	return r.fleets, nil
}

func (r *fleetReadRepositoryStub) ListForCompanyMember(
	_ context.Context,
	userID string,
) ([]*models.Fleet, error) {
	r.memberListCalls++
	r.memberUserID = userID

	if r.err != nil {
		return nil, r.err
	}
	return r.fleets, nil
}

func (r *fleetReadRepositoryStub) ListActiveByCompanyAndBranch(
	context.Context,
	string,
	string,
) ([]*models.Fleet, error) {
	return nil, nil
}

func (r *fleetReadRepositoryStub) Update(
	context.Context,
	*models.Fleet,
) error {
	return nil
}

func (r *fleetReadRepositoryStub) Delete(
	context.Context,
	string,
) error {
	return nil
}

type fleetReadUserRoleRepositoryStub struct {
	roles []string
	err   error
}

func (r *fleetReadUserRoleRepositoryStub) AssignRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *fleetReadUserRoleRepositoryStub) RemoveRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *fleetReadUserRoleRepositoryStub) UserHasRole(
	context.Context,
	string,
	string,
) (bool, error) {
	return false, nil
}

func (r *fleetReadUserRoleRepositoryStub) GetUserRoles(
	context.Context,
	string,
) ([]string, error) {
	return r.roles, r.err
}

func TestGetByIDUsesMembershipScopedReadForNonSystemAdmin(t *testing.T) {
	fleetRepo := &fleetReadRepositoryStub{
		fleet: &models.Fleet{
			CompanyID: "company-1",
			BranchID:  "branch-1",
			Code:      "FLEET-001",
			Name:      "Fleet One",
			IsActive:  true,
		},
	}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		UserRoles: &fleetReadUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
	})

	result, err := service.GetByID(
		context.Background(),
		"user-1",
		"fleet-1",
	)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}

	if result.Code != "FLEET-001" {
		t.Fatalf("expected FLEET-001, got %q", result.Code)
	}
	if fleetRepo.memberGetCalls != 1 {
		t.Fatalf(
			"expected 1 membership-scoped get, got %d",
			fleetRepo.memberGetCalls,
		)
	}
	if fleetRepo.globalGetCalls != 0 {
		t.Fatalf(
			"non-system admin must not use global get, got %d calls",
			fleetRepo.globalGetCalls,
		)
	}
	if fleetRepo.memberUserID != "user-1" {
		t.Fatalf(
			"expected membership read for user-1, got %q",
			fleetRepo.memberUserID,
		)
	}
}

func TestGetByIDUsesGlobalReadForSystemAdmin(t *testing.T) {
	fleetRepo := &fleetReadRepositoryStub{
		fleet: &models.Fleet{
			Code:     "FLEET-001",
			Name:     "Fleet One",
			IsActive: true,
		},
	}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		UserRoles: &fleetReadUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	_, err := service.GetByID(
		context.Background(),
		"system-admin",
		"fleet-1",
	)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}

	if fleetRepo.globalGetCalls != 1 {
		t.Fatalf(
			"expected 1 global get, got %d",
			fleetRepo.globalGetCalls,
		)
	}
	if fleetRepo.memberGetCalls != 0 {
		t.Fatalf(
			"system admin must not use membership get, got %d calls",
			fleetRepo.memberGetCalls,
		)
	}
}

func TestGetByIDPreservesNotFoundForCrossTenantRead(t *testing.T) {
	fleetRepo := &fleetReadRepositoryStub{
		err: repository.ErrNotFound,
	}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		UserRoles: &fleetReadUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
	})

	_, err := service.GetByID(
		context.Background(),
		"user-1",
		"cross-tenant-fleet",
	)
	if !errors.Is(err, ErrFleetNotFound) {
		t.Fatalf(
			"expected ErrFleetNotFound, got %v",
			err,
		)
	}

	if fleetRepo.memberGetCalls != 1 {
		t.Fatalf(
			"expected membership-scoped get, got %d calls",
			fleetRepo.memberGetCalls,
		)
	}
	if fleetRepo.globalGetCalls != 0 {
		t.Fatal("cross-tenant read must not fall back to global get")
	}
}

func TestListUsesMembershipScopedReadForNonSystemAdmin(t *testing.T) {
	fleetRepo := &fleetReadRepositoryStub{
		fleets: []*models.Fleet{
			{
				CompanyID: "company-1",
				Code:      "FLEET-001",
				Name:      "Fleet One",
			},
		},
	}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		UserRoles: &fleetReadUserRoleRepositoryStub{
			roles: []string{"DISPATCHER"},
		},
	})

	result, err := service.List(
		context.Background(),
		"user-1",
	)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 fleet, got %d", len(result))
	}
	if fleetRepo.memberListCalls != 1 {
		t.Fatalf(
			"expected 1 membership-scoped list, got %d",
			fleetRepo.memberListCalls,
		)
	}
	if fleetRepo.globalListCalls != 0 {
		t.Fatalf(
			"non-system admin must not use global list, got %d calls",
			fleetRepo.globalListCalls,
		)
	}
	if fleetRepo.memberUserID != "user-1" {
		t.Fatalf(
			"expected membership list for user-1, got %q",
			fleetRepo.memberUserID,
		)
	}
}

func TestListUsesGlobalReadForSystemAdmin(t *testing.T) {
	fleetRepo := &fleetReadRepositoryStub{
		fleets: []*models.Fleet{
			{Code: "FLEET-001"},
			{Code: "FLEET-002"},
		},
	}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		UserRoles: &fleetReadUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	result, err := service.List(
		context.Background(),
		"system-admin",
	)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 fleets, got %d", len(result))
	}
	if fleetRepo.globalListCalls != 1 {
		t.Fatalf(
			"expected 1 global list, got %d",
			fleetRepo.globalListCalls,
		)
	}
	if fleetRepo.memberListCalls != 0 {
		t.Fatalf(
			"system admin must not use membership list, got %d calls",
			fleetRepo.memberListCalls,
		)
	}
}

var _ repository.FleetRepository = (*fleetReadRepositoryStub)(nil)
var _ repository.UserRoleRepository = (*fleetReadUserRoleRepositoryStub)(nil)
