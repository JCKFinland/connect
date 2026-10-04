package vehicle

import (
	"context"
	"errors"
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

func TestCanCreateVehicleForCompanyAllowsCompanyMember(
	t *testing.T,
) {
	service := NewService(Dependencies{
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
		CompanyMemberships: &vehicleCompanyMembershipRepositoryStub{
			memberships: map[string]map[string]bool{
				"user-1": {"company-1": true},
			},
		},
	})

	authorized, err := service.canCreateVehicleForCompany(
		context.Background(),
		"user-1",
		"company-1",
	)
	if err != nil {
		t.Fatalf("check company vehicle creation authority: %v", err)
	}

	if !authorized {
		t.Fatal("expected explicit company member to be authorized")
	}
}

func TestCanCreateVehicleForCompanyRejectsCrossTenantMembership(
	t *testing.T,
) {
	service := NewService(Dependencies{
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
		CompanyMemberships: &vehicleCompanyMembershipRepositoryStub{
			memberships: map[string]map[string]bool{
				"user-1": {"company-1": true},
			},
		},
	})

	authorized, err := service.canCreateVehicleForCompany(
		context.Background(),
		"user-1",
		"company-2",
	)
	if err != nil {
		t.Fatalf("check cross-tenant vehicle creation authority: %v", err)
	}

	if authorized {
		t.Fatal("cross-tenant company membership must not authorize creation")
	}
}

func TestCanCreateVehicleForCompanyAllowsSystemAdminWithoutMembership(
	t *testing.T,
) {
	service := NewService(Dependencies{
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
		CompanyMemberships: &vehicleCompanyMembershipRepositoryStub{},
	})

	authorized, err := service.canCreateVehicleForCompany(
		context.Background(),
		"system-admin",
		"company-1",
	)
	if err != nil {
		t.Fatalf("check system admin vehicle creation authority: %v", err)
	}

	if !authorized {
		t.Fatal("SYSTEM_ADMIN must bypass company membership")
	}
}

func TestCanCreateVehicleForCompanyPropagatesMembershipFailure(
	t *testing.T,
) {
	expected := errors.New("membership lookup failed")

	service := NewService(Dependencies{
		UserRoles: &vehicleUserRoleRepositoryStub{
			roles: []string{"COMPANY_ADMIN"},
		},
		CompanyMemberships: &vehicleCompanyMembershipRepositoryStub{
			err: expected,
		},
	})

	authorized, err := service.canCreateVehicleForCompany(
		context.Background(),
		"user-1",
		"company-1",
	)
	if authorized {
		t.Fatal("membership failure must not authorize creation")
	}

	if !errors.Is(err, expected) {
		t.Fatalf(
			"expected membership repository failure, got %v",
			err,
		)
	}
}

var _ repository.UserRoleRepository = (*vehicleUserRoleRepositoryStub)(nil)
var _ repository.CompanyMembershipRepository = (*vehicleCompanyMembershipRepositoryStub)(nil)
