package pricing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

type fakeFarePricingProfileRepository struct {
	resolveResult *models.FarePricingProfile
	resolveErr    error

	gotCompanyID         string
	gotBranchID          *string
	gotServiceCategoryID string
	gotAt                time.Time
}

func (f *fakeFarePricingProfileRepository) Create(
	ctx context.Context,
	profile *models.FarePricingProfile,
) error {
	return nil
}

func (f *fakeFarePricingProfileRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.FarePricingProfile, error) {
	return nil, nil
}

func (f *fakeFarePricingProfileRepository) GetByVersion(
	ctx context.Context,
	companyID string,
	version string,
) (*models.FarePricingProfile, error) {
	return nil, nil
}

func (f *fakeFarePricingProfileRepository) ResolveEffective(
	ctx context.Context,
	companyID string,
	branchID *string,
	serviceCategoryID string,
	at time.Time,
) (*models.FarePricingProfile, error) {
	f.gotCompanyID = companyID
	f.gotBranchID = branchID
	f.gotServiceCategoryID = serviceCategoryID
	f.gotAt = at

	return f.resolveResult, f.resolveErr
}

func (f *fakeFarePricingProfileRepository) ListByCompanyID(
	ctx context.Context,
	companyID string,
	activeOnly bool,
) ([]*models.FarePricingProfile, error) {
	return nil, nil
}

func TestResolveValidation(
	t *testing.T,
) {
	at := time.Now().UTC()

	tests := []struct {
		name    string
		input   ResolveInput
		wantErr error
	}{
		{
			name: "missing company",
			input: ResolveInput{
				ServiceCategoryID: "category-1",
				At:                at,
			},
			wantErr: ErrInvalidCompanyID,
		},
		{
			name: "missing service category",
			input: ResolveInput{
				CompanyID: "company-1",
				At:        at,
			},
			wantErr: ErrInvalidServiceCategoryID,
		},
		{
			name: "missing effective time",
			input: ResolveInput{
				CompanyID:         "company-1",
				ServiceCategoryID: "category-1",
			},
			wantErr: ErrInvalidEffectiveTime,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				repo :=
					&fakeFarePricingProfileRepository{}

				svc := NewService(
					Dependencies{
						FarePricingProfiles: repo,
					},
				)

				_, err := svc.Resolve(
					context.Background(),
					tt.input,
				)

				if !errors.Is(
					err,
					tt.wantErr,
				) {
					t.Fatalf(
						"expected error %v, got %v",
						tt.wantErr,
						err,
					)
				}
			},
		)
	}
}

func TestResolveMapsProfileToPricingSnapshot(
	t *testing.T,
) {
	at := time.Date(
		2026,
		time.August,
		29,
		12,
		30,
		0,
		0,
		time.UTC,
	)

	branchID := "branch-1"

	repo := &fakeFarePricingProfileRepository{
		resolveResult: &models.FarePricingProfile{
			ID:                "pricing-profile-1",
			CompanyID:         "company-1",
			BranchID:          &branchID,
			ServiceCategoryID: "category-1",
			Version:           "v42",
			Currency:          "EUR",

			BaseFare:             6.50,
			DistanceRatePerKM:    1.95,
			TimeRatePerMinute:    0.70,
			WaitingRatePerMinute: 0.55,
			BookingFee:           2.25,
			SurgeMultiplier:      1.30,

			EffectiveFrom: at.Add(-time.Hour),

			IsActive: true,
		},
	}

	svc := NewService(
		Dependencies{
			FarePricingProfiles: repo,
		},
	)

	result, err := svc.Resolve(
		context.Background(),
		ResolveInput{
			CompanyID:         "company-1",
			BranchID:          &branchID,
			ServiceCategoryID: "category-1",
			At:                at,
		},
	)
	if err != nil {
		t.Fatalf(
			"resolve pricing: %v",
			err,
		)
	}

	if repo.gotCompanyID != "company-1" {
		t.Fatalf(
			"expected company-1, got %s",
			repo.gotCompanyID,
		)
	}

	if repo.gotBranchID == nil ||
		*repo.gotBranchID != branchID {
		t.Fatalf(
			"unexpected branch ID: %v",
			repo.gotBranchID,
		)
	}

	if repo.gotServiceCategoryID !=
		"category-1" {
		t.Fatalf(
			"expected category-1, got %s",
			repo.gotServiceCategoryID,
		)
	}

	if !repo.gotAt.Equal(at) {
		t.Fatalf(
			"expected resolver time %v, got %v",
			at,
			repo.gotAt,
		)
	}

	if result.ProfileID !=
		"pricing-profile-1" {
		t.Fatalf(
			"unexpected profile ID: %s",
			result.ProfileID,
		)
	}

	if result.ServiceCategoryID !=
		"category-1" {
		t.Fatalf(
			"unexpected category ID: %s",
			result.ServiceCategoryID,
		)
	}

	pricing := result.Pricing

	if pricing.BaseFare != 6.50 {
		t.Fatalf(
			"expected base fare 6.50, got %v",
			pricing.BaseFare,
		)
	}

	if pricing.DistanceRatePerKM != 1.95 {
		t.Fatalf(
			"expected distance rate 1.95, got %v",
			pricing.DistanceRatePerKM,
		)
	}

	if pricing.TimeRatePerMinute != 0.70 {
		t.Fatalf(
			"expected time rate 0.70, got %v",
			pricing.TimeRatePerMinute,
		)
	}

	if pricing.WaitingRatePerMinute != 0.55 {
		t.Fatalf(
			"expected waiting rate 0.55, got %v",
			pricing.WaitingRatePerMinute,
		)
	}

	if pricing.BookingFee != 2.25 {
		t.Fatalf(
			"expected booking fee 2.25, got %v",
			pricing.BookingFee,
		)
	}

	if pricing.SurgeMultiplier != 1.30 {
		t.Fatalf(
			"expected surge multiplier 1.30, got %v",
			pricing.SurgeMultiplier,
		)
	}

	if pricing.Currency != "EUR" {
		t.Fatalf(
			"expected EUR, got %s",
			pricing.Currency,
		)
	}

	if pricing.PricingVersion != "v42" {
		t.Fatalf(
			"expected v42, got %s",
			pricing.PricingVersion,
		)
	}

	if pricing.DiscountAmount != 0 {
		t.Fatalf(
			"expected zero discount, got %v",
			pricing.DiscountAmount,
		)
	}

	if pricing.TaxAmount != 0 {
		t.Fatalf(
			"expected zero tax, got %v",
			pricing.TaxAmount,
		)
	}

	if pricing.TollAmount != 0 {
		t.Fatalf(
			"expected zero toll, got %v",
			pricing.TollAmount,
		)
	}

	if pricing.ParkingAmount != 0 {
		t.Fatalf(
			"expected zero parking amount, got %v",
			pricing.ParkingAmount,
		)
	}
}

func TestResolveRepositoryError(
	t *testing.T,
) {
	repositoryErr :=
		errors.New("database unavailable")

	repo := &fakeFarePricingProfileRepository{
		resolveErr: repositoryErr,
	}

	svc := NewService(
		Dependencies{
			FarePricingProfiles: repo,
		},
	)

	_, err := svc.Resolve(
		context.Background(),
		ResolveInput{
			CompanyID:         "company-1",
			ServiceCategoryID: "category-1",
			At:                time.Now().UTC(),
		},
	)

	if err == nil {
		t.Fatal(
			"expected repository error",
		)
	}

	if !errors.Is(
		err,
		repositoryErr,
	) {
		t.Fatalf(
			"expected wrapped repository error, got %v",
			err,
		)
	}
}

func TestResolvePricingProfileNotFound(
	t *testing.T,
) {
	repo := &fakeFarePricingProfileRepository{
		resolveErr: pgx.ErrNoRows,
	}

	svc := NewService(
		Dependencies{
			FarePricingProfiles: repo,
		},
	)

	_, err := svc.Resolve(
		context.Background(),
		ResolveInput{
			CompanyID:         "company-1",
			ServiceCategoryID: "category-1",
			At:                time.Now().UTC(),
		},
	)

	if err == nil {
		t.Fatal(
			"expected pricing profile not found error",
		)
	}

	if !errors.Is(
		err,
		ErrPricingProfileNotFound,
	) {
		t.Fatalf(
			"expected ErrPricingProfileNotFound, got %v",
			err,
		)
	}

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		t.Fatal(
			"pgx.ErrNoRows must not leak from pricing service",
		)
	}
}
