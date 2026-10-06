package fleet

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type listForDriverDriverRepoStub struct {
	driver *models.Driver
	err    error
}

func (r *listForDriverFleetRepoStub) Deactivate(context.Context, string) error { return nil }
func (r *listForDriverFleetRepoStub) DeactivateForCompanyMember(context.Context, string, string) error {
	return nil
}
func (r *listForDriverFleetRepoStub) Reactivate(context.Context, string) error { return nil }
func (r *listForDriverFleetRepoStub) ReactivateForCompanyMember(context.Context, string, string) error {
	return nil
}
func (r *listForDriverFleetRepoStub) IsOwningBranchActive(context.Context, string) (bool, error) {
	return true, nil
}

func (s *listForDriverDriverRepoStub) Create(
	context.Context,
	*models.Driver,
) error {
	return nil
}

func (s *listForDriverDriverRepoStub) GetByID(
	context.Context,
	string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (s *listForDriverDriverRepoStub) GetByIDForUpdate(
	context.Context,
	string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (s *listForDriverDriverRepoStub) GetByUserID(
	_ context.Context,
	_ string,
) (*models.Driver, error) {
	return s.driver, s.err
}

func (s *listForDriverDriverRepoStub) List(
	context.Context,
) ([]models.Driver, error) {
	return nil, nil
}

func (s *listForDriverDriverRepoStub) Update(
	context.Context,
	*models.Driver,
) error {
	return nil
}

func (s *listForDriverDriverRepoStub) Delete(
	context.Context,
	string,
) error {
	return nil
}

type listForDriverFleetRepoStub struct {
	fleets []*models.Fleet
	err    error

	companyID string
	branchID  string
}

func (s *listForDriverFleetRepoStub) Create(
	context.Context,
	*models.Fleet,
) error {
	return nil
}

func (s *listForDriverFleetRepoStub) GetByID(
	context.Context,
	string,
) (*models.Fleet, error) {
	return nil, repository.ErrNotFound
}

func (s *listForDriverFleetRepoStub) GetByIDForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) (*models.Fleet, error) {
	return s.GetByID(ctx, id)
}

func (s *listForDriverFleetRepoStub) List(
	context.Context,
) ([]*models.Fleet, error) {
	return nil, nil
}

func (s *listForDriverFleetRepoStub) ListForCompanyMember(
	ctx context.Context,
	userID string,
) ([]*models.Fleet, error) {
	return s.List(ctx)
}

func (s *listForDriverFleetRepoStub) ListActiveByCompanyAndBranch(
	_ context.Context,
	companyID string,
	branchID string,
) ([]*models.Fleet, error) {
	s.companyID = companyID
	s.branchID = branchID

	return s.fleets, s.err
}

func (s *listForDriverFleetRepoStub) UpdateDetails(
	ctx context.Context,
	fleet *models.Fleet,
) error {
	return nil
}

func (s *listForDriverFleetRepoStub) UpdateDetailsForCompanyMember(
	ctx context.Context,
	userID string,
	fleet *models.Fleet,
) error {
	return nil
}

func (s *listForDriverFleetRepoStub) Archive(
	context.Context,
	string,
) error {
	return nil
}

func (s *listForDriverFleetRepoStub) ArchiveForCompanyMember(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *listForDriverFleetRepoStub) GetOwningBranchID(
	context.Context,
	string,
) (string, error) {
	return "", repository.ErrNotFound
}

func (r *listForDriverFleetRepoStub) HasNonDeletedByBranch(
	context.Context,
	string,
) (bool, error) {
	return false, nil
}

func (r *listForDriverFleetRepoStub) HasActiveByBranch(
	context.Context,
	string,
) (bool, error) {
	return false, nil
}

func TestListForDriverUsesDriverTenantScope(t *testing.T) {
	driverRepo := &listForDriverDriverRepoStub{
		driver: &models.Driver{
			UserID:     "user-1",
			CompanyID:  "company-1",
			BranchID:   "branch-1",
			Status:     "ACTIVE",
			IsVerified: true,
			IsActive:   true,
		},
	}

	fleetRepo := &listForDriverFleetRepoStub{
		fleets: []*models.Fleet{
			{
				CompanyID:   "company-1",
				BranchID:    "branch-1",
				Code:        "FLEET-001",
				Name:        "Main Fleet",
				Description: "Driver fleet",
				IsActive:    true,
			},
		},
	}

	service := NewService(Dependencies{
		Fleets:  fleetRepo,
		Drivers: driverRepo,
	})

	result, err := service.ListForDriver(
		context.Background(),
		"user-1",
	)
	if err != nil {
		t.Fatalf("ListForDriver returned error: %v", err)
	}

	if fleetRepo.companyID != "company-1" {
		t.Fatalf(
			"expected company scope company-1, got %q",
			fleetRepo.companyID,
		)
	}

	if fleetRepo.branchID != "branch-1" {
		t.Fatalf(
			"expected branch scope branch-1, got %q",
			fleetRepo.branchID,
		)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 fleet, got %d", len(result))
	}

	if result[0].Code != "FLEET-001" {
		t.Fatalf(
			"expected fleet code FLEET-001, got %q",
			result[0].Code,
		)
	}
}

func TestListForDriverRejectsUnverifiedDriver(t *testing.T) {
	service := NewService(Dependencies{
		Fleets: &listForDriverFleetRepoStub{},
		Drivers: &listForDriverDriverRepoStub{
			driver: &models.Driver{
				UserID:     "user-1",
				CompanyID:  "company-1",
				BranchID:   "branch-1",
				Status:     "ACTIVE",
				IsVerified: false,
				IsActive:   true,
			},
		},
	})

	_, err := service.ListForDriver(
		context.Background(),
		"user-1",
	)

	if !errors.Is(err, ErrDriverNotEligible) {
		t.Fatalf(
			"expected ErrDriverNotEligible, got %v",
			err,
		)
	}
}

func TestListForDriverRejectsMissingDriver(t *testing.T) {
	service := NewService(Dependencies{
		Fleets: &listForDriverFleetRepoStub{},
		Drivers: &listForDriverDriverRepoStub{
			err: repository.ErrNotFound,
		},
	})

	_, err := service.ListForDriver(
		context.Background(),
		"user-1",
	)

	if !errors.Is(err, ErrDriverNotEligible) {
		t.Fatalf(
			"expected ErrDriverNotEligible, got %v",
			err,
		)
	}
}

func TestListForDriverPropagatesFleetRepositoryError(t *testing.T) {
	expectedErr := errors.New("fleet repository failure")

	service := NewService(Dependencies{
		Fleets: &listForDriverFleetRepoStub{
			err: expectedErr,
		},
		Drivers: &listForDriverDriverRepoStub{
			driver: &models.Driver{
				UserID:     "user-1",
				CompanyID:  "company-1",
				BranchID:   "branch-1",
				Status:     "ACTIVE",
				IsVerified: true,
				IsActive:   true,
			},
		},
	})

	_, err := service.ListForDriver(
		context.Background(),
		"user-1",
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected fleet repository error, got %v",
			err,
		)
	}
}
