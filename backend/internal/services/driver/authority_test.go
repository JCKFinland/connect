package driver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type managementDriverRepositoryStub struct {
	driver           *models.Driver
	getErr           error
	updatedDetails   *models.Driver
	deletedID        string
	updateDetailsErr error
}

func (r *managementDriverRepositoryStub) Create(
	context.Context,
	*models.Driver,
) error {
	return nil
}

func (r *managementDriverRepositoryStub) GetByID(
	_ context.Context,
	id string,
) (*models.Driver, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}

	if r.driver == nil || r.driver.ID != id {
		return nil, repository.ErrNotFound
	}

	copy := *r.driver
	return &copy, nil
}

func (r *managementDriverRepositoryStub) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.Driver, error) {
	return r.GetByID(ctx, id)
}

func (r *managementDriverRepositoryStub) GetByUserID(
	context.Context,
	string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (r *managementDriverRepositoryStub) List(
	context.Context,
) ([]models.Driver, error) {
	return nil, nil
}

func (r *managementDriverRepositoryStub) UpdateDetails(
	_ context.Context,
	driver *models.Driver,
) error {
	copy := *driver
	r.updatedDetails = &copy
	return r.updateDetailsErr
}

func (r *managementDriverRepositoryStub) Update(
	context.Context,
	*models.Driver,
) error {
	return nil
}

func (r *managementDriverRepositoryStub) Delete(
	_ context.Context,
	id string,
) error {
	r.deletedID = id
	return nil
}

type managementUserRoleRepositoryStub struct {
	roles []string
	err   error
}

func (r *managementUserRoleRepositoryStub) AssignRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *managementUserRoleRepositoryStub) RemoveRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *managementUserRoleRepositoryStub) UserHasRole(
	context.Context,
	string,
	string,
) (bool, error) {
	return false, nil
}

func (r *managementUserRoleRepositoryStub) GetUserRoles(
	context.Context,
	string,
) ([]string, error) {
	return r.roles, r.err
}

type managementCompanyMembershipRepositoryStub struct {
	member        bool
	err           error
	checkedUserID string
	checkedCoID   string
}

func (r *managementCompanyMembershipRepositoryStub) Exists(
	_ context.Context,
	userID string,
	companyID string,
) (bool, error) {
	r.checkedUserID = userID
	r.checkedCoID = companyID
	return r.member, r.err
}

func (r *managementCompanyMembershipRepositoryStub) Create(
	context.Context,
	*models.CompanyMembership,
) error {
	return nil
}

func (r *managementCompanyMembershipRepositoryStub) ListByUserID(
	context.Context,
	string,
) ([]*models.CompanyMembership, error) {
	return nil, nil
}

func managementDriverFixture() *models.Driver {
	expiry := time.Date(2030, 6, 1, 0, 0, 0, 0, time.UTC)
	verifiedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	return &models.Driver{
		BaseModel: models.BaseModel{
			ID: "driver-record-1",
		},
		UserID:                  "driver-user-1",
		CompanyID:               "company-1",
		BranchID:                "branch-1",
		DriverNumber:            "OLD-001",
		FirstName:               "Old",
		LastName:                "Name",
		Phone:                   "+358400000001",
		Email:                   "old@example.com",
		TaxiDriverLicenseNumber: "TAXI-ORIGINAL",
		DrivingLicenseNumber:    "DL-ORIGINAL",
		DrivingLicenseExpiry:    &expiry,
		Status:                  "ACTIVE",
		IsVerified:              true,
		VerifiedAt:              &verifiedAt,
		VerifiedByUserID:        stringPointer("verifier-1"),
		IsActive:                true,
	}
}

func stringPointer(value string) *string {
	return &value
}

func TestUpdateAllowsCompanyMemberAndPreservesAuthorityLifecycleFields(
	t *testing.T,
) {
	repo := &managementDriverRepositoryStub{
		driver: managementDriverFixture(),
	}
	roles := &managementUserRoleRepositoryStub{}
	memberships := &managementCompanyMembershipRepositoryStub{
		member: true,
	}

	service := NewService(Dependencies{
		Drivers:            repo,
		UserRoles:          roles,
		CompanyMemberships: memberships,
	})

	hireDate := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)

	err := service.Update(
		context.Background(),
		"manager-1",
		"driver-record-1",
		UpdateDriverRequest{
			DriverNumber: "NEW-001",
			FirstName:    "New",
			LastName:     "Profile",
			Phone:        "+358400000002",
			Email:        "new@example.com",
			HireDate:     &hireDate,
		},
	)
	if err != nil {
		t.Fatalf("update driver: %v", err)
	}

	if memberships.checkedUserID != "manager-1" ||
		memberships.checkedCoID != "company-1" {
		t.Fatalf(
			"unexpected membership check: user=%q company=%q",
			memberships.checkedUserID,
			memberships.checkedCoID,
		)
	}

	got := repo.updatedDetails
	if got == nil {
		t.Fatal("expected UpdateDetails to be called")
	}

	if got.DriverNumber != "NEW-001" ||
		got.FirstName != "New" ||
		got.LastName != "Profile" ||
		got.Phone != "+358400000002" ||
		got.Email != "new@example.com" {
		t.Fatalf("profile details were not updated: %#v", got)
	}

	if got.CompanyID != "company-1" ||
		got.BranchID != "branch-1" ||
		got.UserID != "driver-user-1" {
		t.Fatalf("authority identity mutated: %#v", got)
	}

	if got.TaxiDriverLicenseNumber != "TAXI-ORIGINAL" ||
		got.DrivingLicenseNumber != "DL-ORIGINAL" {
		t.Fatalf("regulatory credentials mutated: %#v", got)
	}

	if got.Status != "ACTIVE" ||
		!got.IsVerified ||
		!got.IsActive ||
		got.VerifiedAt == nil ||
		got.VerifiedByUserID == nil ||
		*got.VerifiedByUserID != "verifier-1" {
		t.Fatalf("verification/lifecycle state mutated: %#v", got)
	}
}

func TestUpdateConcealsCrossTenantDriver(t *testing.T) {
	repo := &managementDriverRepositoryStub{
		driver: managementDriverFixture(),
	}
	service := NewService(Dependencies{
		Drivers:   repo,
		UserRoles: &managementUserRoleRepositoryStub{},
		CompanyMemberships: &managementCompanyMembershipRepositoryStub{
			member: false,
		},
	})

	err := service.Update(
		context.Background(),
		"other-company-manager",
		"driver-record-1",
		UpdateDriverRequest{},
	)

	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}

	if repo.updatedDetails != nil {
		t.Fatal("cross-tenant update reached persistence")
	}
}

func TestDeleteConcealsCrossTenantDriver(t *testing.T) {
	repo := &managementDriverRepositoryStub{
		driver: managementDriverFixture(),
	}
	service := NewService(Dependencies{
		Drivers:   repo,
		UserRoles: &managementUserRoleRepositoryStub{},
		CompanyMemberships: &managementCompanyMembershipRepositoryStub{
			member: false,
		},
	})

	err := service.Delete(
		context.Background(),
		"other-company-manager",
		"driver-record-1",
	)

	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}

	if repo.deletedID != "" {
		t.Fatal("cross-tenant delete reached persistence")
	}
}

func TestSystemAdminCanManageDriverWithoutCompanyMembership(t *testing.T) {
	repo := &managementDriverRepositoryStub{
		driver: managementDriverFixture(),
	}
	memberships := &managementCompanyMembershipRepositoryStub{
		member: false,
	}

	service := NewService(Dependencies{
		Drivers: repo,
		UserRoles: &managementUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
		CompanyMemberships: memberships,
	})

	err := service.Update(
		context.Background(),
		"system-admin-1",
		"driver-record-1",
		UpdateDriverRequest{
			DriverNumber: "ADMIN-001",
			FirstName:    "Admin",
			LastName:     "Updated",
		},
	)
	if err != nil {
		t.Fatalf("system admin update: %v", err)
	}

	if repo.updatedDetails == nil {
		t.Fatal("expected system admin update to reach persistence")
	}

	if memberships.checkedUserID != "" || memberships.checkedCoID != "" {
		t.Fatal("system admin should bypass company membership lookup")
	}
}

func TestManagementTargetConcealsNonexistentDriver(t *testing.T) {
	repo := &managementDriverRepositoryStub{}

	service := NewService(Dependencies{
		Drivers: repo,
		UserRoles: &managementUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	err := service.Update(
		context.Background(),
		"system-admin-1",
		"missing-driver",
		UpdateDriverRequest{},
	)

	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf("expected ErrDriverNotFound, got %v", err)
	}
}

func TestUpdateConcealsConcurrentArchive(t *testing.T) {
	repo := &managementDriverRepositoryStub{
		driver:           managementDriverFixture(),
		updateDetailsErr: repository.ErrNotFound,
	}

	service := NewService(Dependencies{
		Drivers:   repo,
		UserRoles: &managementUserRoleRepositoryStub{},
		CompanyMemberships: &managementCompanyMembershipRepositoryStub{
			member: true,
		},
	})

	err := service.Update(
		context.Background(),
		"manager-1",
		"driver-record-1",
		UpdateDriverRequest{
			FirstName: "Too Late",
		},
	)

	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf(
			"expected ErrDriverNotFound after concurrent archive, got %v",
			err,
		)
	}
}
