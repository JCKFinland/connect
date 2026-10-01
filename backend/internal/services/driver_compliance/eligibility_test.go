package driver_compliance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type complianceDriverRepository struct {
	driver *models.Driver
	err    error
}

func (r *complianceDriverRepository) Create(
	context.Context,
	*models.Driver,
) error {
	return nil
}

func (r *complianceDriverRepository) GetByID(
	context.Context,
	string,
) (*models.Driver, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.driver, nil
}

func (r *complianceDriverRepository) GetByIDForUpdate(
	context.Context,
	string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (r *complianceDriverRepository) GetByUserID(
	context.Context,
	string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (r *complianceDriverRepository) List(
	context.Context,
) ([]models.Driver, error) {
	return []models.Driver{}, nil
}

func (r *complianceDriverRepository) Update(
	context.Context,
	*models.Driver,
) error {
	return nil
}

func (r *complianceDriverRepository) Delete(
	context.Context,
	string,
) error {
	return nil
}

type complianceDocumentRepository struct {
	documents map[string]*models.DriverDocument
	err       error
}

func (r *complianceDocumentRepository) Create(
	context.Context,
	*models.DriverDocument,
) error {
	return nil
}

func (r *complianceDocumentRepository) GetByID(
	context.Context,
	string,
) (*models.DriverDocument, error) {
	return nil, repository.ErrNotFound
}

func (r *complianceDocumentRepository) GetByIDForUpdate(
	context.Context,
	string,
) (*models.DriverDocument, error) {
	return nil, repository.ErrNotFound
}

func (r *complianceDocumentRepository) GetByDriverAndType(
	_ context.Context,
	_ string,
	documentType string,
) (*models.DriverDocument, error) {
	if r.err != nil {
		return nil, r.err
	}

	document, ok := r.documents[documentType]
	if !ok {
		return nil, repository.ErrNotFound
	}

	return document, nil
}

func (r *complianceDocumentRepository) ListByDriver(
	context.Context,
	string,
) ([]models.DriverDocument, error) {
	return []models.DriverDocument{}, nil
}

func (r *complianceDocumentRepository) UpdateReviewState(
	context.Context,
	string,
	string,
	*time.Time,
	string,
	*string,
) (time.Time, error) {
	return time.Time{}, nil
}

func (r *complianceDocumentRepository) UpdateRevocationState(
	context.Context,
	string,
	time.Time,
	string,
	string,
) (time.Time, error) {
	return time.Time{}, nil
}

func (r *complianceDocumentRepository) SoftDelete(
	context.Context,
	string,
) error {
	return nil
}

func TestEvaluateEligibleDriver(t *testing.T) {
	now := time.Date(
		2026,
		time.September,
		28,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	driverExpiry := now.AddDate(1, 0, 0)
	documentExpiry := now.AddDate(1, 0, 0)

	service := newEligibleComplianceService(
		now,
		driverExpiry,
		documentExpiry,
	)

	result, err := service.Evaluate(
		context.Background(),
		"driver-123",
		now,
	)
	if err != nil {
		t.Fatalf("evaluate eligible driver: %v", err)
	}

	if result == nil {
		t.Fatal("expected eligibility result")
	}

	if !result.Eligible {
		t.Fatalf(
			"expected driver to be eligible, reasons: %v",
			result.Reasons,
		)
	}

	if len(result.Reasons) != 0 {
		t.Fatalf(
			"expected no ineligibility reasons, got %v",
			result.Reasons,
		)
	}
}

func TestEvaluateRejectsNonCompliantDriverState(t *testing.T) {
	now := time.Date(
		2026,
		time.September,
		28,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	future := now.AddDate(1, 0, 0)

	tests := []struct {
		name       string
		mutate     func(*models.Driver)
		wantReason string
	}{
		{
			name: "inactive driver",
			mutate: func(driver *models.Driver) {
				driver.IsActive = false
			},
			wantReason: ReasonDriverInactive,
		},
		{
			name: "unverified driver",
			mutate: func(driver *models.Driver) {
				driver.IsVerified = false
			},
			wantReason: ReasonDriverNotVerified,
		},
		{
			name: "driver status not active",
			mutate: func(driver *models.Driver) {
				driver.Status = "PENDING_VERIFICATION"
			},
			wantReason: ReasonDriverStatusNotActive,
		},
		{
			name: "driving license expiry missing",
			mutate: func(driver *models.Driver) {
				driver.DrivingLicenseExpiry = nil
			},
			wantReason: ReasonDrivingLicenseExpired,
		},
		{
			name: "driving license expired",
			mutate: func(driver *models.Driver) {
				expired := now.Add(-time.Second)
				driver.DrivingLicenseExpiry = &expired
			},
			wantReason: ReasonDrivingLicenseExpired,
		},
		{
			name: "driving license expires exactly now",
			mutate: func(driver *models.Driver) {
				expiresNow := now
				driver.DrivingLicenseExpiry = &expiresNow
			},
			wantReason: ReasonDrivingLicenseExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := eligibleDriver(future)
			tt.mutate(driver)

			service := NewService(Dependencies{
				Drivers: &complianceDriverRepository{
					driver: driver,
				},
				Documents: eligibleDocumentRepository(future),
			})

			result, err := service.Evaluate(
				context.Background(),
				driver.ID,
				now,
			)
			if err != nil {
				t.Fatalf("evaluate driver: %v", err)
			}

			assertIneligibleWithReason(
				t,
				result,
				tt.wantReason,
			)
		})
	}
}

func TestEvaluateRejectsNonCompliantDocuments(t *testing.T) {
	now := time.Date(
		2026,
		time.September,
		28,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	future := now.AddDate(1, 0, 0)

	tests := []struct {
		name       string
		mutate     func(*complianceDocumentRepository)
		wantReason string
	}{
		{
			name: "driving license document missing",
			mutate: func(repo *complianceDocumentRepository) {
				delete(
					repo.documents,
					models.DriverDocumentTypeDrivingLicense,
				)
			},
			wantReason: ReasonDrivingLicenseMissing,
		},
		{
			name: "driving license document pending",
			mutate: func(repo *complianceDocumentRepository) {
				repo.documents[models.DriverDocumentTypeDrivingLicense].Status = models.DriverDocumentStatusPending
			},
			wantReason: ReasonDrivingLicenseNotVerified,
		},
		{
			name: "driving license document revoked",
			mutate: func(repo *complianceDocumentRepository) {
				repo.documents[models.DriverDocumentTypeDrivingLicense].Status = models.DriverDocumentStatusRevoked
			},
			wantReason: ReasonDrivingLicenseNotVerified,
		},
		{
			name: "driving license document expired",
			mutate: func(repo *complianceDocumentRepository) {
				expired := now.Add(-time.Second)
				repo.documents[models.DriverDocumentTypeDrivingLicense].ExpiresAt = &expired
			},
			wantReason: ReasonDrivingLicenseDocExpired,
		},
		{
			name: "taxi driver license document missing",
			mutate: func(repo *complianceDocumentRepository) {
				delete(
					repo.documents,
					models.DriverDocumentTypeTaxiDriverLicense,
				)
			},
			wantReason: ReasonTaxiLicenseMissing,
		},
		{
			name: "taxi driver license document rejected",
			mutate: func(repo *complianceDocumentRepository) {
				repo.documents[models.DriverDocumentTypeTaxiDriverLicense].Status = models.DriverDocumentStatusRejected
			},
			wantReason: ReasonTaxiLicenseNotVerified,
		},
		{
			name: "taxi driver license expiry missing",
			mutate: func(repo *complianceDocumentRepository) {
				repo.documents[models.DriverDocumentTypeTaxiDriverLicense].ExpiresAt = nil
			},
			wantReason: ReasonTaxiLicenseExpired,
		},
		{
			name: "taxi driver license expires exactly now",
			mutate: func(repo *complianceDocumentRepository) {
				expiresNow := now
				repo.documents[models.DriverDocumentTypeTaxiDriverLicense].ExpiresAt = &expiresNow
			},
			wantReason: ReasonTaxiLicenseExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			documents := eligibleDocumentRepository(future)
			tt.mutate(documents)

			service := NewService(Dependencies{
				Drivers: &complianceDriverRepository{
					driver: eligibleDriver(future),
				},
				Documents: documents,
			})

			result, err := service.Evaluate(
				context.Background(),
				"driver-123",
				now,
			)
			if err != nil {
				t.Fatalf("evaluate driver: %v", err)
			}

			assertIneligibleWithReason(
				t,
				result,
				tt.wantReason,
			)
		})
	}
}

func TestEvaluateMissingDriverIsIneligible(t *testing.T) {
	service := NewService(Dependencies{
		Drivers: &complianceDriverRepository{
			err: repository.ErrNotFound,
		},
		Documents: &complianceDocumentRepository{},
	})

	result, err := service.Evaluate(
		context.Background(),
		"driver-123",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("evaluate missing driver: %v", err)
	}

	assertIneligibleWithReason(
		t,
		result,
		ReasonDriverInactive,
	)
}

func TestEvaluateReturnsDriverRepositoryFailure(t *testing.T) {
	repositoryFailure := errors.New("driver database unavailable")

	service := NewService(Dependencies{
		Drivers: &complianceDriverRepository{
			err: repositoryFailure,
		},
		Documents: &complianceDocumentRepository{},
	})

	_, err := service.Evaluate(
		context.Background(),
		"driver-123",
		time.Now().UTC(),
	)
	if !errors.Is(err, repositoryFailure) {
		t.Fatalf(
			"expected driver repository failure, got %v",
			err,
		)
	}
}

func TestEvaluateReturnsDocumentRepositoryFailure(t *testing.T) {
	now := time.Now().UTC()
	future := now.AddDate(1, 0, 0)

	documentFailure := errors.New("document database unavailable")

	service := NewService(Dependencies{
		Drivers: &complianceDriverRepository{
			driver: eligibleDriver(future),
		},
		Documents: &complianceDocumentRepository{
			err: documentFailure,
		},
	})

	_, err := service.Evaluate(
		context.Background(),
		"driver-123",
		now,
	)
	if !errors.Is(err, documentFailure) {
		t.Fatalf(
			"expected document repository failure, got %v",
			err,
		)
	}
}

func eligibleDriver(
	expiresAt time.Time,
) *models.Driver {
	return &models.Driver{
		BaseModel: models.BaseModel{
			ID: "driver-123",
		},
		Status:               "ACTIVE",
		IsVerified:           true,
		IsActive:             true,
		DrivingLicenseExpiry: &expiresAt,
	}
}

func eligibleDocumentRepository(
	expiresAt time.Time,
) *complianceDocumentRepository {
	return &complianceDocumentRepository{
		documents: map[string]*models.DriverDocument{
			models.DriverDocumentTypeDrivingLicense: {
				BaseModel: models.BaseModel{
					ID: "driving-license-document",
				},
				DriverID:     "driver-123",
				DocumentType: models.DriverDocumentTypeDrivingLicense,
				Status:       models.DriverDocumentStatusVerified,
				ExpiresAt:    &expiresAt,
			},
			models.DriverDocumentTypeTaxiDriverLicense: {
				BaseModel: models.BaseModel{
					ID: "taxi-license-document",
				},
				DriverID:     "driver-123",
				DocumentType: models.DriverDocumentTypeTaxiDriverLicense,
				Status:       models.DriverDocumentStatusVerified,
				ExpiresAt:    &expiresAt,
			},
		},
	}
}

func newEligibleComplianceService(
	now time.Time,
	driverExpiry time.Time,
	documentExpiry time.Time,
) *Service {
	_ = now

	return NewService(Dependencies{
		Drivers: &complianceDriverRepository{
			driver: eligibleDriver(driverExpiry),
		},
		Documents: eligibleDocumentRepository(documentExpiry),
	})
}

func assertIneligibleWithReason(
	t *testing.T,
	result *Eligibility,
	wantReason string,
) {
	t.Helper()

	if result == nil {
		t.Fatal("expected eligibility result")
	}

	if result.Eligible {
		t.Fatalf(
			"expected driver to be ineligible, got eligible result",
		)
	}

	for _, reason := range result.Reasons {
		if reason == wantReason {
			return
		}
	}

	t.Fatalf(
		"expected reason %q, got %v",
		wantReason,
		result.Reasons,
	)
}
