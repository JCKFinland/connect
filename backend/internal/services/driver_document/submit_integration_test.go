package driver_document

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestSubmitDocumentCreatesAndReplacesActiveDocument(
	t *testing.T,
) {
	ctx := context.Background()

	db := openDriverDocumentTestDB(t)
	defer db.Close()

	fixture, cleanupFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf(
			"create driver fixture: %v",
			err,
		)
	}

	defer func() {
		// Driver documents reference drivers, so remove the document
		// history before cleaning up the driver hierarchy.
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup driver documents: %v",
				cleanupErr,
			)
		}

		if cleanupErr := cleanupFixture(
			context.Background(),
		); cleanupErr != nil {
			t.Logf(
				"cleanup driver fixture: %v",
				cleanupErr,
			)
		}
	}()

	documentRepo :=
		postgresrepo.NewDriverDocumentRepository(db)

	driverRepo :=
		postgresrepo.NewDriverRepository(db)

	service := NewService(
		Dependencies{
			DB:        db,
			Documents: documentRepo,
			Drivers:   driverRepo,
		},
	)

	firstExpiry := time.Now().
		UTC().
		AddDate(1, 0, 0)

	first, err := service.Submit(
		ctx,
		fixture.DriverID,
		SubmitDocumentRequest{
			DocumentType: models.DriverDocumentTypeTaxiDriverLicense,
			FileName:     "taxi-driver-license-v1.pdf",
			StorageKey: "drivers/" +
				fixture.DriverID +
				"/taxi-driver-license-v1.pdf",
			ContentType:   "application/pdf",
			FileSizeBytes: 4096,
			ExpiresAt:     &firstExpiry,
		},
	)
	if err != nil {
		t.Fatalf(
			"submit first driver document: %v",
			err,
		)
	}

	if first == nil {
		t.Fatal("expected first document")
	}

	if first.ID == "" {
		t.Fatal("expected first document ID")
	}

	if first.DriverID != fixture.DriverID {
		t.Fatalf(
			"expected driver ID %s, got %s",
			fixture.DriverID,
			first.DriverID,
		)
	}

	if first.Status !=
		models.DriverDocumentStatusPending {
		t.Fatalf(
			"expected first status %s, got %s",
			models.DriverDocumentStatusPending,
			first.Status,
		)
	}

	if first.VerifiedAt != nil ||
		first.VerifiedByUserID != nil ||
		first.RejectionReason != nil {
		t.Fatal(
			"expected first submission to have empty verification state",
		)
	}

	persistedFirst, err :=
		documentRepo.GetByDriverAndType(
			ctx,
			fixture.DriverID,
			models.DriverDocumentTypeTaxiDriverLicense,
		)
	if err != nil {
		t.Fatalf(
			"load first active document: %v",
			err,
		)
	}

	if persistedFirst.ID != first.ID {
		t.Fatalf(
			"expected active document ID %s, got %s",
			first.ID,
			persistedFirst.ID,
		)
	}

	secondExpiry := time.Now().
		UTC().
		AddDate(2, 0, 0)

	second, err := service.Submit(
		ctx,
		fixture.DriverID,
		SubmitDocumentRequest{
			DocumentType: models.DriverDocumentTypeTaxiDriverLicense,
			FileName:     "taxi-driver-license-v2.pdf",
			StorageKey: "drivers/" +
				fixture.DriverID +
				"/taxi-driver-license-v2.pdf",
			ContentType:   "application/pdf",
			FileSizeBytes: 8192,
			ExpiresAt:     &secondExpiry,
		},
	)
	if err != nil {
		t.Fatalf(
			"replace driver document: %v",
			err,
		)
	}

	if second == nil {
		t.Fatal("expected replacement document")
	}

	if second.ID == "" {
		t.Fatal("expected replacement document ID")
	}

	if second.ID == first.ID {
		t.Fatalf(
			"expected replacement ID to differ from first ID %s",
			first.ID,
		)
	}

	if second.Status !=
		models.DriverDocumentStatusPending {
		t.Fatalf(
			"expected replacement status %s, got %s",
			models.DriverDocumentStatusPending,
			second.Status,
		)
	}

	active, err :=
		documentRepo.GetByDriverAndType(
			ctx,
			fixture.DriverID,
			models.DriverDocumentTypeTaxiDriverLicense,
		)
	if err != nil {
		t.Fatalf(
			"load replacement active document: %v",
			err,
		)
	}

	if active.ID != second.ID {
		t.Fatalf(
			"expected replacement %s to be active, got %s",
			second.ID,
			active.ID,
		)
	}

	var (
		historyCount int
		activeCount  int
		oldDeletedAt *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				COUNT(*),
				COUNT(*) FILTER (
					WHERE deleted_at IS NULL
				)
			FROM driver_documents
			WHERE driver_id = $1
			  AND document_type = $2
		`,
		fixture.DriverID,
		models.DriverDocumentTypeTaxiDriverLicense,
	).Scan(
		&historyCount,
		&activeCount,
	); err != nil {
		t.Fatalf(
			"count document history: %v",
			err,
		)
	}

	if historyCount != 2 {
		t.Fatalf(
			"expected 2 historical documents, got %d",
			historyCount,
		)
	}

	if activeCount != 1 {
		t.Fatalf(
			"expected exactly 1 active document, got %d",
			activeCount,
		)
	}

	if err := db.QueryRow(
		ctx,
		`
			SELECT deleted_at
			FROM driver_documents
			WHERE id = $1
		`,
		first.ID,
	).Scan(
		&oldDeletedAt,
	); err != nil {
		t.Fatalf(
			"load original deleted_at: %v",
			err,
		)
	}

	if oldDeletedAt == nil {
		t.Fatal(
			"expected original document to be soft-deleted",
		)
	}

	_, err = documentRepo.GetByID(
		ctx,
		first.ID,
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected original document hidden after replacement, got %v",
			err,
		)
	}
}

func TestSubmitDocumentReturnsDriverNotFound(
	t *testing.T,
) {
	ctx := context.Background()

	db := openDriverDocumentTestDB(t)
	defer db.Close()

	service := NewService(
		Dependencies{
			DB:        db,
			Documents: postgresrepo.NewDriverDocumentRepository(db),
			Drivers:   postgresrepo.NewDriverRepository(db),
		},
	)

	expiry := time.Now().
		UTC().
		AddDate(1, 0, 0)

	document, err := service.Submit(
		ctx,
		uuid.NewString(),
		SubmitDocumentRequest{
			DocumentType:  models.DriverDocumentTypeDrivingLicense,
			FileName:      "driving-license.pdf",
			StorageKey:    "drivers/missing/driving-license.pdf",
			ContentType:   "application/pdf",
			FileSizeBytes: 2048,
			ExpiresAt:     &expiry,
		},
	)

	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf(
			"expected ErrDriverNotFound, got %v",
			err,
		)
	}

	if document != nil {
		t.Fatalf(
			"expected nil document, got %+v",
			document,
		)
	}
}

func openDriverDocumentTestDB(
	t *testing.T,
) *pgxpool.Pool {
	t.Helper()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf(
			"get working directory: %v",
			err,
		)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf(
			"change to backend root: %v",
			err,
		)
	}

	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Fatalf(
				"restore working directory: %v",
				err,
			)
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect database: %v",
			err,
		)
	}

	return db
}

func TestSubmitDocumentRollsBackWhenReplacementCreateFails(
	t *testing.T,
) {
	ctx := context.Background()

	db := openDriverDocumentTestDB(t)
	defer db.Close()

	fixture, cleanupFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf(
			"create driver fixture: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, cleanupErr := db.Exec(
			cleanupCtx,
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup driver documents: %v",
				cleanupErr,
			)
		}

		if cleanupErr := cleanupFixture(
			cleanupCtx,
		); cleanupErr != nil {
			t.Logf(
				"cleanup driver fixture: %v",
				cleanupErr,
			)
		}
	}()

	documentRepo :=
		postgresrepo.NewDriverDocumentRepository(db)

	service := NewService(
		Dependencies{
			DB:        db,
			Documents: documentRepo,
			Drivers:   postgresrepo.NewDriverRepository(db),
		},
	)

	originalExpiry := time.Now().
		UTC().
		AddDate(1, 0, 0)

	original, err := service.Submit(
		ctx,
		fixture.DriverID,
		SubmitDocumentRequest{
			DocumentType: models.DriverDocumentTypeDrivingLicense,
			FileName:     "driving-license-original.pdf",
			StorageKey: "drivers/" +
				fixture.DriverID +
				"/driving-license-original.pdf",
			ContentType:   "application/pdf",
			FileSizeBytes: 4096,
			ExpiresAt:     &originalExpiry,
		},
	)
	if err != nil {
		t.Fatalf(
			"submit original document: %v",
			err,
		)
	}

	if original == nil || original.ID == "" {
		t.Fatal(
			"expected persisted original document",
		)
	}

	replacementExpiry := time.Now().
		UTC().
		AddDate(2, 0, 0)

	// PostgreSQL text values cannot contain a NUL byte.
	//
	// The request passes service-level nonempty validation, allowing the
	// transaction to:
	//   1. lock the driver,
	//   2. soft-delete the original document,
	//   3. attempt the replacement INSERT,
	//   4. receive a PostgreSQL persistence error.
	//
	// RunInTransaction must then roll back the soft delete as well.
	invalidStorageKey :=
		"drivers/" +
			fixture.DriverID +
			"/invalid\x00replacement.pdf"

	replacement, err := service.Submit(
		ctx,
		fixture.DriverID,
		SubmitDocumentRequest{
			DocumentType:  models.DriverDocumentTypeDrivingLicense,
			FileName:      "driving-license-replacement.pdf",
			StorageKey:    invalidStorageKey,
			ContentType:   "application/pdf",
			FileSizeBytes: 8192,
			ExpiresAt:     &replacementExpiry,
		},
	)

	if err == nil {
		t.Fatal(
			"expected replacement persistence failure",
		)
	}

	if replacement != nil {
		t.Fatalf(
			"expected nil replacement after persistence failure, got %+v",
			replacement,
		)
	}

	active, err :=
		documentRepo.GetByDriverAndType(
			ctx,
			fixture.DriverID,
			models.DriverDocumentTypeDrivingLicense,
		)
	if err != nil {
		t.Fatalf(
			"load active document after rollback: %v",
			err,
		)
	}

	if active.ID != original.ID {
		t.Fatalf(
			"expected original document %s to remain active, got %s",
			original.ID,
			active.ID,
		)
	}

	var (
		deletedAt    *time.Time
		historyCount int
		activeCount  int
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT deleted_at
			FROM driver_documents
			WHERE id = $1
		`,
		original.ID,
	).Scan(
		&deletedAt,
	); err != nil {
		t.Fatalf(
			"load original deleted_at after rollback: %v",
			err,
		)
	}

	if deletedAt != nil {
		t.Fatalf(
			"expected rollback to restore original active document, deleted_at=%v",
			deletedAt,
		)
	}

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				COUNT(*),
				COUNT(*) FILTER (
					WHERE deleted_at IS NULL
				)
			FROM driver_documents
			WHERE driver_id = $1
			  AND document_type = $2
		`,
		fixture.DriverID,
		models.DriverDocumentTypeDrivingLicense,
	).Scan(
		&historyCount,
		&activeCount,
	); err != nil {
		t.Fatalf(
			"count documents after rollback: %v",
			err,
		)
	}

	if historyCount != 1 {
		t.Fatalf(
			"expected failed replacement not to persist, history count=%d",
			historyCount,
		)
	}

	if activeCount != 1 {
		t.Fatalf(
			"expected exactly one active original document, got %d",
			activeCount,
		)
	}
}

func TestGetAndListDocumentsEnforceDriverOwnership(
	t *testing.T,
) {
	db := openDriverDocumentTestDB(t)
	t.Cleanup(db.Close)

	ctx := context.Background()

	ownerFixture, ownerCleanup, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf(
			"create owner driver fixture: %v",
			err,
		)
	}
	t.Cleanup(func() {
		ownerCleanup(context.Background())
	})

	otherFixture, otherCleanup, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf(
			"create other driver fixture: %v",
			err,
		)
	}
	t.Cleanup(func() {
		otherCleanup(context.Background())
	})

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = ANY($1::uuid[])
			`,
			[]string{
				ownerFixture.DriverID,
				otherFixture.DriverID,
			},
		)
		if err != nil {
			t.Errorf(
				"cleanup driver documents: %v",
				err,
			)
		}
	})

	documentRepo :=
		postgresrepo.NewDriverDocumentRepository(db)

	service := NewService(
		Dependencies{
			DB:        db,
			Documents: documentRepo,
		},
	)

	expiry := time.Now().
		UTC().
		AddDate(1, 0, 0)

	ownerDocument := &models.DriverDocument{
		DriverID:     ownerFixture.DriverID,
		DocumentType: models.DriverDocumentTypeDrivingLicense,
		FileName:     "owner-driving-license.pdf",
		StorageKey: "drivers/" +
			ownerFixture.DriverID +
			"/driving-license.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 4096,
		ExpiresAt:     &expiry,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := documentRepo.Create(
		ctx,
		ownerDocument,
	); err != nil {
		t.Fatalf(
			"create owner document: %v",
			err,
		)
	}

	got, err := service.Get(
		ctx,
		ownerFixture.DriverID,
		ownerDocument.ID,
	)
	if err != nil {
		t.Fatalf(
			"owner get document: %v",
			err,
		)
	}

	if got == nil {
		t.Fatal("expected owner document")
	}

	if got.ID != ownerDocument.ID ||
		got.DriverID != ownerFixture.DriverID {
		t.Fatalf(
			"unexpected owner document: %+v",
			got,
		)
	}

	crossDriverDocument, err := service.Get(
		ctx,
		otherFixture.DriverID,
		ownerDocument.ID,
	)
	if !errors.Is(
		err,
		ErrDocumentNotFound,
	) {
		t.Fatalf(
			"expected ErrDocumentNotFound for cross-driver access, got %v",
			err,
		)
	}

	if crossDriverDocument != nil {
		t.Fatalf(
			"expected nil cross-driver document, got %+v",
			crossDriverDocument,
		)
	}

	ownerDocuments, err := service.List(
		ctx,
		ownerFixture.DriverID,
	)
	if err != nil {
		t.Fatalf(
			"list owner documents: %v",
			err,
		)
	}

	if len(ownerDocuments) != 1 {
		t.Fatalf(
			"expected 1 owner document, got %d",
			len(ownerDocuments),
		)
	}

	if ownerDocuments[0].ID !=
		ownerDocument.ID {
		t.Fatalf(
			"expected owner document ID %s, got %s",
			ownerDocument.ID,
			ownerDocuments[0].ID,
		)
	}

	otherDocuments, err := service.List(
		ctx,
		otherFixture.DriverID,
	)
	if err != nil {
		t.Fatalf(
			"list other driver documents: %v",
			err,
		)
	}

	if otherDocuments == nil {
		t.Fatal(
			"expected non-nil empty document slice",
		)
	}

	if len(otherDocuments) != 0 {
		t.Fatalf(
			"expected other driver to have no documents, got %+v",
			otherDocuments,
		)
	}
}
