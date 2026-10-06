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

func (r *fleetCreateRepositoryStub) Deactivate(context.Context, string) error { return nil }
func (r *fleetCreateRepositoryStub) DeactivateForCompanyMember(context.Context, string, string) error {
	return nil
}
func (r *fleetCreateRepositoryStub) Reactivate(context.Context, string) error { return nil }
func (r *fleetCreateRepositoryStub) ReactivateForCompanyMember(context.Context, string, string) error {
	return nil
}
func (r *fleetCreateRepositoryStub) GetOwningBranchID(context.Context, string) (string, error) {
	return "", repository.ErrNotFound
}

func (r *fleetCreateRepositoryStub) HasNonDeletedByBranch(context.Context, string) (bool, error) {
	return false, nil
}

func (r *fleetCreateRepositoryStub) HasActiveByBranch(context.Context, string) (bool, error) {
	return false, nil
}

func (r *fleetCreateRepositoryStub) IsOwningBranchActive(context.Context, string) (bool, error) {
	return true, nil
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

func (r *fleetCreateRepositoryStub) GetByIDForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) (*models.Fleet, error) {
	return r.GetByID(ctx, id)
}

func (r *fleetCreateRepositoryStub) List(
	context.Context,
) ([]*models.Fleet, error) {
	return nil, nil
}

func (r *fleetCreateRepositoryStub) ListForCompanyMember(
	ctx context.Context,
	userID string,
) ([]*models.Fleet, error) {
	return r.List(ctx)
}

func (r *fleetCreateRepositoryStub) ListActiveByCompanyAndBranch(
	context.Context,
	string,
	string,
) ([]*models.Fleet, error) {
	return nil, nil
}

func (r *fleetCreateRepositoryStub) UpdateDetails(
	ctx context.Context,
	fleet *models.Fleet,
) error {
	return nil
}

func (r *fleetCreateRepositoryStub) UpdateDetailsForCompanyMember(
	ctx context.Context,
	userID string,
	fleet *models.Fleet,
) error {
	return nil
}

func (r *fleetCreateRepositoryStub) Archive(
	context.Context,
	string,
) error {
	return nil
}

func (r *fleetCreateRepositoryStub) ArchiveForCompanyMember(
	context.Context,
	string,
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

func (r *fleetCreateBranchRepositoryStub) UpdateDetails(
	context.Context,
	*models.Branch,
) error {
	return nil
}

func (r *fleetCreateBranchRepositoryStub) UpdateDetailsForCompanyMember(
	context.Context,
	string,
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

func (r *fleetCreateBranchRepositoryStub) GetByIDForCompanyMember(
	ctx context.Context,
	_ string,
	id string,
) (*models.Branch, error) {
	return r.GetByID(ctx, id)
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

func (r *fleetCreateBranchRepositoryStub) Archive(
	context.Context,
	string,
) error {
	return nil
}

func (r *fleetCreateBranchRepositoryStub) ArchiveForCompanyMember(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *fleetCreateBranchRepositoryStub) Deactivate(
	context.Context,
	string,
) error {
	return nil
}

func (r *fleetCreateBranchRepositoryStub) DeactivateForCompanyMember(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *fleetCreateBranchRepositoryStub) Reactivate(
	context.Context,
	string,
) error {
	return nil
}

func (r *fleetCreateBranchRepositoryStub) ReactivateForCompanyMember(
	context.Context,
	string,
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

func (r *fleetCreateBranchRepositoryStub) ListForCompanyMember(
	ctx context.Context,
	userID string,
) ([]*models.Branch, error) {
	return nil, nil
}

var _ repository.UserRoleRepository = (*fleetCreateUserRoleRepositoryStub)(nil)
var _ repository.CompanyMembershipRepository = (*fleetCreateCompanyMembershipRepositoryStub)(nil)
