package fleet

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type fleetCreateRepositoryStub struct {
	created *models.Fleet
	err     error
}

func (r *fleetCreateRepositoryStub) Create(
	_ context.Context,
	fleet *models.Fleet,
) error {
	if r.err != nil {
		return r.err
	}

	r.created = fleet
	return nil
}

func (r *fleetCreateRepositoryStub) GetByID(
	context.Context,
	string,
) (*models.Fleet, error) {
	return nil, repository.ErrNotFound
}

func (r *fleetCreateRepositoryStub) List(
	context.Context,
) ([]*models.Fleet, error) {
	return nil, nil
}

func (r *fleetCreateRepositoryStub) ListActiveByCompanyAndBranch(
	context.Context,
	string,
	string,
) ([]*models.Fleet, error) {
	return nil, nil
}

func (r *fleetCreateRepositoryStub) Update(
	context.Context,
	*models.Fleet,
) error {
	return nil
}

func (r *fleetCreateRepositoryStub) Delete(
	context.Context,
	string,
) error {
	return nil
}

type fleetCreateBranchRepositoryStub struct {
	branch *models.Branch
	err    error
}

func (r *fleetCreateBranchRepositoryStub) Create(
	context.Context,
	*models.Branch,
) error {
	return nil
}

func (r *fleetCreateBranchRepositoryStub) Update(
	context.Context,
	*models.Branch,
) error {
	return nil
}

func (r *fleetCreateBranchRepositoryStub) GetByID(
	context.Context,
	string,
) (*models.Branch, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.branch, nil
}

func (r *fleetCreateBranchRepositoryStub) List(
	context.Context,
) ([]*models.Branch, error) {
	return nil, nil
}

func (r *fleetCreateBranchRepositoryStub) ListActiveByCompanyID(
	context.Context,
	string,
) ([]*models.Branch, error) {
	return nil, nil
}

func (r *fleetCreateBranchRepositoryStub) Delete(
	context.Context,
	string,
) error {
	return nil
}

type fleetCreateUserRoleRepositoryStub struct {
	roles []string
	err   error
}

func (r *fleetCreateUserRoleRepositoryStub) AssignRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *fleetCreateUserRoleRepositoryStub) RemoveRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *fleetCreateUserRoleRepositoryStub) UserHasRole(
	context.Context,
	string,
	string,
) (bool, error) {
	return false, nil
}

func (r *fleetCreateUserRoleRepositoryStub) GetUserRoles(
	context.Context,
	string,
) ([]string, error) {
	return r.roles, r.err
}

type fleetCreateCompanyMembershipRepositoryStub struct {
	memberships map[string]map[string]bool
	err         error
}

func (r *fleetCreateCompanyMembershipRepositoryStub) Create(
	context.Context,
	*models.CompanyMembership,
) error {
	return nil
}

func (r *fleetCreateCompanyMembershipRepositoryStub) Exists(
	_ context.Context,
	userID string,
	companyID string,
) (bool, error) {
	if r.err != nil {
		return false, r.err
	}

	companies := r.memberships[userID]
	return companies != nil && companies[companyID], nil
}

func (r *fleetCreateCompanyMembershipRepositoryStub) ListByUserID(
	context.Context,
	string,
) ([]*models.CompanyMembership, error) {
	return nil, nil
}

func TestCreateDerivesCompanyFromBranchAndCreatesActiveFleet(t *testing.T) {
	fleetRepo := &fleetCreateRepositoryStub{}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		Branches: &fleetCreateBranchRepositoryStub{
			branch: &models.Branch{
				BaseModel: models.BaseModel{ID: "branch-1"},
				CompanyID: "company-1",
				IsActive:  true,
			},
		},
		UserRoles: &fleetCreateUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
		CompanyMemberships: &fleetCreateCompanyMembershipRepositoryStub{
			memberships: map[string]map[string]bool{
				"user-1": {"company-1": true},
			},
		},
	})

	result, err := service.Create(
		context.Background(),
		"user-1",
		CreateFleetRequest{
			BranchID:    "branch-1",
			Code:        "FLEET-001",
			Name:        "Main Fleet",
			Description: "Primary fleet",
		},
	)
	if err != nil {
		t.Fatalf("create fleet: %v", err)
	}

	if fleetRepo.created == nil {
		t.Fatal("expected fleet to be created")
	}

	if fleetRepo.created.CompanyID != "company-1" {
		t.Fatalf(
			"expected company derived from branch, got %q",
			fleetRepo.created.CompanyID,
		)
	}

	if fleetRepo.created.BranchID != "branch-1" {
		t.Fatalf(
			"expected authoritative branch branch-1, got %q",
			fleetRepo.created.BranchID,
		)
	}

	if !fleetRepo.created.IsActive {
		t.Fatal("expected service-owned initial active state")
	}

	if result.CompanyID != "company-1" ||
		result.BranchID != "branch-1" ||
		!result.IsActive {
		t.Fatalf("unexpected response authority fields: %#v", result)
	}
}

func TestCreateRejectsCrossTenantMembership(t *testing.T) {
	fleetRepo := &fleetCreateRepositoryStub{}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		Branches: &fleetCreateBranchRepositoryStub{
			branch: &models.Branch{
				BaseModel: models.BaseModel{ID: "branch-2"},
				CompanyID: "company-2",
				IsActive:  true,
			},
		},
		UserRoles: &fleetCreateUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
		CompanyMemberships: &fleetCreateCompanyMembershipRepositoryStub{
			memberships: map[string]map[string]bool{
				"user-1": {"company-1": true},
			},
		},
	})

	_, err := service.Create(
		context.Background(),
		"user-1",
		CreateFleetRequest{
			BranchID: "branch-2",
			Code:     "FLEET-002",
			Name:     "Other Fleet",
		},
	)

	if !errors.Is(err, ErrFleetCreationAccessDenied) {
		t.Fatalf(
			"expected ErrFleetCreationAccessDenied, got %v",
			err,
		)
	}

	if fleetRepo.created != nil {
		t.Fatal("cross-tenant fleet must not be created")
	}
}

func TestCreateAllowsSystemAdminWithoutCompanyMembership(t *testing.T) {
	fleetRepo := &fleetCreateRepositoryStub{}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		Branches: &fleetCreateBranchRepositoryStub{
			branch: &models.Branch{
				BaseModel: models.BaseModel{ID: "branch-1"},
				CompanyID: "company-1",
				IsActive:  true,
			},
		},
		UserRoles: &fleetCreateUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
		CompanyMemberships: &fleetCreateCompanyMembershipRepositoryStub{},
	})

	_, err := service.Create(
		context.Background(),
		"system-admin",
		CreateFleetRequest{
			BranchID: "branch-1",
			Code:     "FLEET-001",
			Name:     "Main Fleet",
		},
	)
	if err != nil {
		t.Fatalf("system admin create fleet: %v", err)
	}

	if fleetRepo.created == nil {
		t.Fatal("expected system admin fleet creation")
	}
}

func TestCreateRejectsInactiveBranch(t *testing.T) {
	fleetRepo := &fleetCreateRepositoryStub{}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		Branches: &fleetCreateBranchRepositoryStub{
			branch: &models.Branch{
				BaseModel: models.BaseModel{ID: "branch-1"},
				CompanyID: "company-1",
				IsActive:  false,
			},
		},
		UserRoles: &fleetCreateUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
		CompanyMemberships: &fleetCreateCompanyMembershipRepositoryStub{},
	})

	_, err := service.Create(
		context.Background(),
		"system-admin",
		CreateFleetRequest{
			BranchID: "branch-1",
			Code:     "FLEET-001",
			Name:     "Main Fleet",
		},
	)

	if !errors.Is(err, ErrInvalidBranch) {
		t.Fatalf("expected ErrInvalidBranch, got %v", err)
	}

	if fleetRepo.created != nil {
		t.Fatal("fleet must not be created in inactive branch")
	}
}

func TestCreateRejectsMissingBranch(t *testing.T) {
	fleetRepo := &fleetCreateRepositoryStub{}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
		Branches: &fleetCreateBranchRepositoryStub{
			err: repository.ErrNotFound,
		},
		UserRoles: &fleetCreateUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	_, err := service.Create(
		context.Background(),
		"system-admin",
		CreateFleetRequest{
			BranchID: "missing-branch",
			Code:     "FLEET-001",
			Name:     "Main Fleet",
		},
	)

	if !errors.Is(err, ErrInvalidBranch) {
		t.Fatalf("expected ErrInvalidBranch, got %v", err)
	}

	if fleetRepo.created != nil {
		t.Fatal("fleet must not be created for missing branch")
	}
}

func TestCreateRejectsMissingAuthenticatedUser(t *testing.T) {
	fleetRepo := &fleetCreateRepositoryStub{}

	service := NewService(Dependencies{
		Fleets: fleetRepo,
	})

	_, err := service.Create(
		context.Background(),
		"",
		CreateFleetRequest{
			BranchID: "branch-1",
			Code:     "FLEET-001",
			Name:     "Main Fleet",
		},
	)

	if !errors.Is(err, ErrFleetCreationAccessDenied) {
		t.Fatalf(
			"expected ErrFleetCreationAccessDenied, got %v",
			err,
		)
	}

	if fleetRepo.created != nil {
		t.Fatal("fleet must not be created without authenticated user")
	}
}

var _ repository.FleetRepository = (*fleetCreateRepositoryStub)(nil)
var _ repository.BranchRepository = (*fleetCreateBranchRepositoryStub)(nil)
var _ repository.UserRoleRepository = (*fleetCreateUserRoleRepositoryStub)(nil)
var _ repository.CompanyMembershipRepository = (*fleetCreateCompanyMembershipRepositoryStub)(nil)
