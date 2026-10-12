package driver_document

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestVerifyDrivingLicenseSynchronizesDriverExpiry(t *testing.T) {
	db := openDriverDocumentTestDB(t)
	t.Cleanup(db.Close)

	ctx := context.Background()

	fixture, cleanupFixture, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := cleanupFixture(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM driver_documents WHERE driver_id = $1`,
			fixture.DriverID,
		)
		if err != nil {
			t.Errorf("cleanup driver documents: %v", err)
		}
	})

	var originalExpiry *time.Time
	var originalStatus string
	var originalVerified bool
	var originalActive bool

	err = db.QueryRow(
		ctx,
		`SELECT driving_license_expiry, status, is_verified, is_active
		 FROM drivers WHERE id = $1`,
		fixture.DriverID,
	).Scan(
		&originalExpiry,
		&originalStatus,
		&originalVerified,
		&originalActive,
	)
	if err != nil {
		t.Fatalf("load initial driver: %v", err)
	}

	expiry := time.Now().UTC().
		AddDate(2, 0, 0).
		Truncate(24 * time.Hour)

	documentRepo :=
		postgresrepo.NewDriverDocumentRepository(db)

	document := &models.DriverDocument{
		DriverID:     fixture.DriverID,
		DocumentType: models.DriverDocumentTypeDrivingLicense,
		FileName:     "driving-license-expiry-test.pdf",
		StorageKey: "drivers/" + fixture.DriverID +
			"/driving-license-expiry-test.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 4096,
		ExpiresAt:     &expiry,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := documentRepo.Create(ctx, document); err != nil {
		t.Fatalf("create document: %v", err)
	}

	service := NewService(Dependencies{
		DB:        db,
		Documents: documentRepo,
	})

	verified, err := service.Verify(
		ctx,
		document.ID,
		fixture.UserID,
	)
	if err != nil {
		t.Fatalf("verify driving licence: %v", err)
	}

	if verified == nil ||
		verified.Status != models.DriverDocumentStatusVerified {
		t.Fatalf("expected verified document, got %+v", verified)
	}

	if verified.VerifiedAt == nil ||
		verified.VerifiedByUserID == nil ||
		*verified.VerifiedByUserID != fixture.UserID {
		t.Fatalf("verification audit metadata missing: %+v", verified)
	}

	persistedDocument, err := documentRepo.GetByID(
		ctx,
		document.ID,
	)
	if err != nil {
		t.Fatalf("load persisted document: %v", err)
	}

	if persistedDocument.Status !=
		models.DriverDocumentStatusVerified {
		t.Fatalf(
			"expected persisted VERIFIED, got %s",
			persistedDocument.Status,
		)
	}

	var persistedExpiry *time.Time
	var persistedStatus string
	var persistedVerified bool
	var persistedActive bool

	err = db.QueryRow(
		ctx,
		`SELECT driving_license_expiry, status, is_verified, is_active
		 FROM drivers WHERE id = $1`,
		fixture.DriverID,
	).Scan(
		&persistedExpiry,
		&persistedStatus,
		&persistedVerified,
		&persistedActive,
	)
	if err != nil {
		t.Fatalf("load updated driver: %v", err)
	}

	if persistedExpiry == nil ||
		!persistedExpiry.Equal(expiry) {
		t.Fatalf(
			"expected driver expiry %v, got %v",
			expiry,
			persistedExpiry,
		)
	}

	if persistedStatus != originalStatus ||
		persistedVerified != originalVerified ||
		persistedActive != originalActive {
		t.Fatal("verification changed unrelated driver lifecycle fields")
	}

	_ = originalExpiry
}

func TestVerifyDrivingLicenseRejectsInvalidExpiry(
	t *testing.T,
) {
	testCases := []struct {
		name   string
		expiry func() *time.Time
	}{
		{
			name: "missing expiry",
			expiry: func() *time.Time {
				return nil
			},
		},
		{
			name: "expired yesterday",
			expiry: func() *time.Time {
				value := dateOnly(time.Now().UTC()).
					AddDate(0, 0, -1)
				return &value
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := openDriverDocumentTestDB(t)
			t.Cleanup(db.Close)

			ctx := context.Background()

			fixture, cleanupFixture, err :=
				testutil.CreateDriverFixture(ctx, db)
			if err != nil {
				t.Fatalf("create fixture: %v", err)
			}

			t.Cleanup(func() {
				if err := cleanupFixture(context.Background()); err != nil {
					t.Errorf("cleanup fixture: %v", err)
				}
			})

			t.Cleanup(func() {
				_, err := db.Exec(
					context.Background(),
					`DELETE FROM driver_documents WHERE driver_id = $1`,
					fixture.DriverID,
				)
				if err != nil {
					t.Errorf("cleanup documents: %v", err)
				}
			})

			repo := postgresrepo.NewDriverDocumentRepository(db)

			document := &models.DriverDocument{
				DriverID:     fixture.DriverID,
				DocumentType: models.DriverDocumentTypeDrivingLicense,
				FileName:     "invalid-driving-license.pdf",
				StorageKey: "drivers/" + fixture.DriverID +
					"/invalid-driving-license.pdf",
				ContentType:   "application/pdf",
				FileSizeBytes: 4096,
				ExpiresAt:     tc.expiry(),
				Status:        models.DriverDocumentStatusPending,
			}

			if err := repo.Create(ctx, document); err != nil {
				t.Fatalf("create document: %v", err)
			}

			service := NewService(Dependencies{
				DB:        db,
				Documents: repo,
			})

			_, err = service.Verify(
				ctx,
				document.ID,
				fixture.UserID,
			)
			if !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf(
					"expected ErrInvalidDocument, got %v",
					err,
				)
			}

			persisted, err := repo.GetByID(
				ctx,
				document.ID,
			)
			if err != nil {
				t.Fatalf("load document: %v", err)
			}

			if persisted.Status !=
				models.DriverDocumentStatusPending {
				t.Fatalf(
					"expected PENDING after rollback, got %s",
					persisted.Status,
				)
			}

			if persisted.VerifiedAt != nil ||
				persisted.VerifiedByUserID != nil {
				t.Fatal("failed verification persisted audit metadata")
			}
		})
	}
}

func TestOtherDocumentReviewsPreserveDrivingLicenseExpiry(
	t *testing.T,
) {
	testCases := []struct {
		name         string
		documentType string
		reject       bool
	}{
		{
			name:         "verify taxi driver licence",
			documentType: models.DriverDocumentTypeTaxiDriverLicense,
		},
		{
			name:         "reject driving licence",
			documentType: models.DriverDocumentTypeDrivingLicense,
			reject:       true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := openDriverDocumentTestDB(t)
			t.Cleanup(db.Close)

			ctx := context.Background()

			fixture, cleanupFixture, err :=
				testutil.CreateDriverFixture(ctx, db)
			if err != nil {
				t.Fatalf("create fixture: %v", err)
			}

			t.Cleanup(func() {
				if err := cleanupFixture(context.Background()); err != nil {
					t.Errorf("cleanup fixture: %v", err)
				}
			})

			t.Cleanup(func() {
				_, err := db.Exec(
					context.Background(),
					`DELETE FROM driver_documents WHERE driver_id = $1`,
					fixture.DriverID,
				)
				if err != nil {
					t.Errorf("cleanup documents: %v", err)
				}
			})

			originalExpiry := dateOnly(
				time.Now().UTC().AddDate(1, 0, 0),
			)

			_, err = db.Exec(
				ctx,
				`UPDATE drivers
				 SET driving_license_expiry = $2
				 WHERE id = $1`,
				fixture.DriverID,
				originalExpiry,
			)
			if err != nil {
				t.Fatalf("set original expiry: %v", err)
			}

			documentExpiry := dateOnly(
				time.Now().UTC().AddDate(2, 0, 0),
			)

			repo := postgresrepo.NewDriverDocumentRepository(db)

			document := &models.DriverDocument{
				DriverID:     fixture.DriverID,
				DocumentType: tc.documentType,
				FileName:     "review-isolation-test.pdf",
				StorageKey: "drivers/" + fixture.DriverID +
					"/review-isolation-test.pdf",
				ContentType:   "application/pdf",
				FileSizeBytes: 4096,
				ExpiresAt:     &documentExpiry,
				Status:        models.DriverDocumentStatusPending,
			}

			if err := repo.Create(ctx, document); err != nil {
				t.Fatalf("create document: %v", err)
			}

			service := NewService(Dependencies{
				DB:        db,
				Documents: repo,
			})

			if tc.reject {
				_, err = service.Reject(
					ctx,
					document.ID,
					fixture.UserID,
					"document evidence insufficient",
				)
			} else {
				_, err = service.Verify(
					ctx,
					document.ID,
					fixture.UserID,
				)
			}

			if err != nil {
				t.Fatalf("review document: %v", err)
			}

			var persistedExpiry time.Time

			err = db.QueryRow(
				ctx,
				`SELECT driving_license_expiry
				 FROM drivers WHERE id = $1`,
				fixture.DriverID,
			).Scan(&persistedExpiry)
			if err != nil {
				t.Fatalf("load driver expiry: %v", err)
			}

			if !persistedExpiry.Equal(originalExpiry) {
				t.Fatalf(
					"unrelated review changed driving licence expiry: expected %v, got %v",
					originalExpiry,
					persistedExpiry,
				)
			}
		})
	}
}
