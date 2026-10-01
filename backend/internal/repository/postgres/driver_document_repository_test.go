package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestDriverDocumentRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()

	// Run from backend root so config.Load() finds .env.
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanupFixture, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf("create isolated driver fixture: %v", err)
	}

	defer func() {
		if err := cleanupFixture(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	repo := NewDriverDocumentRepository(db)

	expiresAt := time.Now().UTC().
		AddDate(1, 0, 0).
		Truncate(24 * time.Hour)

	document := &models.DriverDocument{
		DriverID:      fixture.DriverID,
		DocumentType:  "TAXI_DRIVER_LICENSE",
		FileName:      "taxi-driver-license.pdf",
		StorageKey:    "driver-documents/test/taxi-driver-license.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 2048,
		ExpiresAt:     &expiresAt,
		Status:        models.DriverDocumentStatusPending,
	}

	// ---------------------------------------------------------
	// 1. Create.
	// ---------------------------------------------------------

	if err := repo.Create(ctx, document); err != nil {
		t.Fatalf("create driver document: %v", err)
	}

	if document.ID == "" {
		t.Fatal("expected document ID to be populated")
	}

	if document.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be populated")
	}

	if document.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be populated")
	}

	documentID := document.ID

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
		); cleanupErr != nil {
			t.Logf("cleanup driver documents: %v", cleanupErr)
		}
	}()

	// ---------------------------------------------------------
	// 2. GetByID round trip.
	// ---------------------------------------------------------

	byID, err := repo.GetByID(ctx, documentID)
	if err != nil {
		t.Fatalf("get driver document by ID: %v", err)
	}

	if byID.ID != documentID {
		t.Fatalf(
			"document ID mismatch: got %s want %s",
			byID.ID,
			documentID,
		)
	}

	if byID.DriverID != fixture.DriverID {
		t.Fatalf(
			"driver ID mismatch: got %s want %s",
			byID.DriverID,
			fixture.DriverID,
		)
	}

	if byID.DocumentType != document.DocumentType {
		t.Fatalf(
			"document type mismatch: got %s want %s",
			byID.DocumentType,
			document.DocumentType,
		)
	}

	if byID.FileName != document.FileName {
		t.Fatalf(
			"file name mismatch: got %s want %s",
			byID.FileName,
			document.FileName,
		)
	}

	if byID.StorageKey != document.StorageKey {
		t.Fatalf(
			"storage key mismatch: got %s want %s",
			byID.StorageKey,
			document.StorageKey,
		)
	}

	if byID.ContentType != document.ContentType {
		t.Fatalf(
			"content type mismatch: got %s want %s",
			byID.ContentType,
			document.ContentType,
		)
	}

	if byID.FileSizeBytes != document.FileSizeBytes {
		t.Fatalf(
			"file size mismatch: got %d want %d",
			byID.FileSizeBytes,
			document.FileSizeBytes,
		)
	}

	if byID.Status != models.DriverDocumentStatusPending {
		t.Fatalf(
			"status mismatch: got %s want %s",
			byID.Status,
			models.DriverDocumentStatusPending,
		)
	}

	if byID.ExpiresAt == nil {
		t.Fatal("expected expires_at")
	}

	if byID.ExpiresAt.Format("2006-01-02") !=
		expiresAt.Format("2006-01-02") {
		t.Fatalf(
			"expiry date mismatch: got %s want %s",
			byID.ExpiresAt.Format("2006-01-02"),
			expiresAt.Format("2006-01-02"),
		)
	}

	// ---------------------------------------------------------
	// 3. GetByDriverAndType.
	// ---------------------------------------------------------

	byType, err := repo.GetByDriverAndType(
		ctx,
		fixture.DriverID,
		"TAXI_DRIVER_LICENSE",
	)
	if err != nil {
		t.Fatalf("get driver document by driver and type: %v", err)
	}

	if byType.ID != documentID {
		t.Fatalf(
			"document ID from type lookup mismatch: got %s want %s",
			byType.ID,
			documentID,
		)
	}

	// ---------------------------------------------------------
	// 4. ListByDriver.
	// ---------------------------------------------------------

	documents, err := repo.ListByDriver(
		ctx,
		fixture.DriverID,
	)
	if err != nil {
		t.Fatalf("list driver documents: %v", err)
	}

	found := false

	for _, item := range documents {
		if item.ID == documentID {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected created document in driver document list")
	}

	// ---------------------------------------------------------
	// 5. Only one active document per driver/type is allowed.
	// ---------------------------------------------------------

	duplicate := &models.DriverDocument{
		DriverID:      fixture.DriverID,
		DocumentType:  "TAXI_DRIVER_LICENSE",
		FileName:      "duplicate-license.pdf",
		StorageKey:    "driver-documents/test/duplicate-license.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 1024,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := repo.Create(ctx, duplicate); err == nil {
		t.Fatal(
			"expected duplicate active driver document type to fail",
		)
	}

	// ---------------------------------------------------------
	// 6. Soft delete.
	// ---------------------------------------------------------

	if err := repo.SoftDelete(ctx, documentID); err != nil {
		t.Fatalf("soft delete driver document: %v", err)
	}

	_, err = repo.GetByID(ctx, documentID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound after soft delete, got %v",
			err,
		)
	}

	_, err = repo.GetByDriverAndType(
		ctx,
		fixture.DriverID,
		"TAXI_DRIVER_LICENSE",
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound from type lookup after soft delete, got %v",
			err,
		)
	}

	documents, err = repo.ListByDriver(
		ctx,
		fixture.DriverID,
	)
	if err != nil {
		t.Fatalf(
			"list driver documents after soft delete: %v",
			err,
		)
	}

	for _, item := range documents {
		if item.ID == documentID {
			t.Fatal(
				"soft-deleted document unexpectedly returned by list",
			)
		}
	}

	// ---------------------------------------------------------
	// 7. Soft deleting the same document again is not found.
	// ---------------------------------------------------------

	err = repo.SoftDelete(ctx, documentID)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound deleting already-deleted document, got %v",
			err,
		)
	}

	// ---------------------------------------------------------
	// 8. A replacement of the same type is allowed after
	// soft deletion because the uniqueness constraint applies
	// only to active records.
	// ---------------------------------------------------------

	replacement := &models.DriverDocument{
		DriverID:      fixture.DriverID,
		DocumentType:  "TAXI_DRIVER_LICENSE",
		FileName:      "replacement-license.pdf",
		StorageKey:    "driver-documents/test/replacement-license.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 3072,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := repo.Create(ctx, replacement); err != nil {
		t.Fatalf(
			"create replacement document after soft delete: %v",
			err,
		)
	}

	if replacement.ID == "" {
		t.Fatal("expected replacement document ID")
	}

	if replacement.ID == documentID {
		t.Fatal(
			"expected replacement document to have a new ID",
		)
	}

	reloadedReplacement, err := repo.GetByDriverAndType(
		ctx,
		fixture.DriverID,
		"TAXI_DRIVER_LICENSE",
	)
	if err != nil {
		t.Fatalf(
			"get replacement document by driver and type: %v",
			err,
		)
	}

	if reloadedReplacement.ID != replacement.ID {
		t.Fatalf(
			"replacement ID mismatch: got %s want %s",
			reloadedReplacement.ID,
			replacement.ID,
		)
	}

	// ---------------------------------------------------------
	// 9. Missing IDs return repository.ErrNotFound.
	// ---------------------------------------------------------

	_, err = repo.GetByID(
		ctx,
		"00000000-0000-0000-0000-000000000000",
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound for missing document, got %v",
			err,
		)
	}

	err = repo.SoftDelete(
		ctx,
		"00000000-0000-0000-0000-000000000000",
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound deleting missing document, got %v",
			err,
		)
	}
}

func TestDriverDocumentRepositoryRevocationPreservesVerificationAudit(
	t *testing.T,
) {
	ctx := context.Background()

	// Run from backend root so config.Load() finds .env.
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanupFixture, err := testutil.CreateDriverFixture(
		ctx,
		db,
	)
	if err != nil {
		t.Fatalf("create isolated driver fixture: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
		); cleanupErr != nil {
			t.Logf("cleanup driver documents: %v", cleanupErr)
		}

		if cleanupErr := cleanupFixture(
			context.Background(),
		); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	}()

	repo := NewDriverDocumentRepository(db)

	expiresAt := time.Now().UTC().
		AddDate(1, 0, 0).
		Truncate(24 * time.Hour)

	document := &models.DriverDocument{
		DriverID:      fixture.DriverID,
		DocumentType:  models.DriverDocumentTypeDrivingLicense,
		FileName:      "driving-license.pdf",
		StorageKey:    "driver-documents/test/driving-license.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 4096,
		ExpiresAt:     &expiresAt,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := repo.Create(ctx, document); err != nil {
		t.Fatalf("create pending driver document: %v", err)
	}

	verifiedAt := time.Now().UTC().Add(-time.Minute)

	if _, err := repo.UpdateReviewState(
		ctx,
		document.ID,
		models.DriverDocumentStatusVerified,
		&verifiedAt,
		fixture.UserID,
		nil,
	); err != nil {
		t.Fatalf("verify driver document: %v", err)
	}

	verified, err := repo.GetByID(ctx, document.ID)
	if err != nil {
		t.Fatalf("reload verified driver document: %v", err)
	}

	if verified.Status != models.DriverDocumentStatusVerified {
		t.Fatalf(
			"verified status mismatch: got %s want %s",
			verified.Status,
			models.DriverDocumentStatusVerified,
		)
	}

	if verified.VerifiedAt == nil {
		t.Fatal("expected verified_at before revocation")
	}

	if verified.VerifiedByUserID == nil {
		t.Fatal("expected verified_by_user_id before revocation")
	}

	originalVerifiedAt := *verified.VerifiedAt
	originalVerifiedByUserID := *verified.VerifiedByUserID

	revokedAt := time.Now().UTC()
	const revocationReason = "credential withdrawn by issuing authority"

	if _, err := repo.UpdateRevocationState(
		ctx,
		document.ID,
		revokedAt,
		fixture.UserID,
		revocationReason,
	); err != nil {
		t.Fatalf("revoke driver document: %v", err)
	}

	revoked, err := repo.GetByID(ctx, document.ID)
	if err != nil {
		t.Fatalf("reload revoked driver document: %v", err)
	}

	if revoked.Status != models.DriverDocumentStatusRevoked {
		t.Fatalf(
			"revoked status mismatch: got %s want %s",
			revoked.Status,
			models.DriverDocumentStatusRevoked,
		)
	}

	if revoked.VerifiedAt == nil {
		t.Fatal("expected verified_at to survive revocation")
	}

	if !revoked.VerifiedAt.Equal(originalVerifiedAt) {
		t.Fatalf(
			"verified_at changed during revocation: got %s want %s",
			revoked.VerifiedAt.UTC(),
			originalVerifiedAt.UTC(),
		)
	}

	if revoked.VerifiedByUserID == nil {
		t.Fatal(
			"expected verified_by_user_id to survive revocation",
		)
	}

	if *revoked.VerifiedByUserID != originalVerifiedByUserID {
		t.Fatalf(
			"verified_by_user_id changed during revocation: got %s want %s",
			*revoked.VerifiedByUserID,
			originalVerifiedByUserID,
		)
	}

	if revoked.RejectionReason != nil {
		t.Fatalf(
			"expected rejection_reason to remain nil, got %q",
			*revoked.RejectionReason,
		)
	}

	if revoked.RevokedAt == nil {
		t.Fatal("expected revoked_at")
	}

	expectedRevokedAt := revokedAt.Truncate(time.Microsecond)

	if !revoked.RevokedAt.Equal(expectedRevokedAt) {
		t.Fatalf(
			"revoked_at mismatch: got %s want %s",
			revoked.RevokedAt.UTC(),
			expectedRevokedAt.UTC(),
		)
	}

	if revoked.RevokedByUserID == nil {
		t.Fatal("expected revoked_by_user_id")
	}

	if *revoked.RevokedByUserID != fixture.UserID {
		t.Fatalf(
			"revoked_by_user_id mismatch: got %s want %s",
			*revoked.RevokedByUserID,
			fixture.UserID,
		)
	}

	if revoked.RevocationReason == nil {
		t.Fatal("expected revocation_reason")
	}

	if *revoked.RevocationReason != revocationReason {
		t.Fatalf(
			"revocation_reason mismatch: got %q want %q",
			*revoked.RevocationReason,
			revocationReason,
		)
	}
}
