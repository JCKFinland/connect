package driver_document

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestVerifyDocumentPersistsVerifiedState(
	t *testing.T,
) {
	db := openDriverDocumentTestDB(t)
	t.Cleanup(db.Close)

	ctx := context.Background()

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

	t.Cleanup(func() {
		cleanupFixture(context.Background())
	})

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
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

	document := &models.DriverDocument{
		DriverID:     fixture.DriverID,
		DocumentType: models.DriverDocumentTypeTaxiDriverLicense,
		FileName:     "taxi-driver-license-review.pdf",
		StorageKey: "drivers/" +
			fixture.DriverID +
			"/taxi-driver-license-review.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 4096,
		ExpiresAt:     &expiry,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := documentRepo.Create(
		ctx,
		document,
	); err != nil {
		t.Fatalf(
			"create pending document: %v",
			err,
		)
	}

	beforeReview := time.Now().UTC()

	verified, err := service.Verify(
		ctx,
		document.ID,
		fixture.UserID,
	)
	if err != nil {
		t.Fatalf(
			"verify document: %v",
			err,
		)
	}

	if verified == nil {
		t.Fatal("expected verified document")
	}

	if verified.Status !=
		models.DriverDocumentStatusVerified {

		t.Fatalf(
			"expected status %s, got %s",
			models.DriverDocumentStatusVerified,
			verified.Status,
		)
	}

	if verified.VerifiedAt == nil {
		t.Fatal(
			"expected verified_at after verification",
		)
	}

	if verified.VerifiedAt.Before(beforeReview) {
		t.Fatalf(
			"expected verified_at >= %v, got %v",
			beforeReview,
			*verified.VerifiedAt,
		)
	}

	if verified.VerifiedByUserID == nil ||
		*verified.VerifiedByUserID != fixture.UserID {

		t.Fatalf(
			"expected reviewer user ID %s, got %v",
			fixture.UserID,
			verified.VerifiedByUserID,
		)
	}

	if verified.RejectionReason != nil {
		t.Fatalf(
			"expected nil rejection reason, got %q",
			*verified.RejectionReason,
		)
	}

	persisted, err := documentRepo.GetByID(
		ctx,
		document.ID,
	)
	if err != nil {
		t.Fatalf(
			"load verified document: %v",
			err,
		)
	}

	if persisted.Status !=
		models.DriverDocumentStatusVerified {

		t.Fatalf(
			"expected persisted status %s, got %s",
			models.DriverDocumentStatusVerified,
			persisted.Status,
		)
	}

	if persisted.VerifiedAt == nil {
		t.Fatal(
			"expected persisted verified_at",
		)
	}

	if persisted.VerifiedByUserID == nil ||
		*persisted.VerifiedByUserID != fixture.UserID {

		t.Fatalf(
			"expected persisted reviewer %s, got %v",
			fixture.UserID,
			persisted.VerifiedByUserID,
		)
	}

	if persisted.RejectionReason != nil {
		t.Fatalf(
			"expected persisted nil rejection reason, got %q",
			*persisted.RejectionReason,
		)
	}

	if !verified.UpdatedAt.Equal(
		persisted.UpdatedAt,
	) {
		t.Fatalf(
			"expected returned updated_at %v to match persisted %v",
			verified.UpdatedAt,
			persisted.UpdatedAt,
		)
	}
}

func TestRejectDocumentPersistsRejectedState(
	t *testing.T,
) {
	db := openDriverDocumentTestDB(t)
	t.Cleanup(db.Close)

	ctx := context.Background()

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

	t.Cleanup(func() {
		cleanupFixture(context.Background())
	})

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
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

	document := &models.DriverDocument{
		DriverID:     fixture.DriverID,
		DocumentType: models.DriverDocumentTypeDrivingLicense,
		FileName:     "driving-license-review.pdf",
		StorageKey: "drivers/" +
			fixture.DriverID +
			"/driving-license-review.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 4096,
		ExpiresAt:     &expiry,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := documentRepo.Create(
		ctx,
		document,
	); err != nil {
		t.Fatalf(
			"create pending document: %v",
			err,
		)
	}

	rejected, err := service.Reject(
		ctx,
		document.ID,
		fixture.UserID,
		"  document image is unreadable  ",
	)
	if err != nil {
		t.Fatalf(
			"reject document: %v",
			err,
		)
	}

	if rejected == nil {
		t.Fatal("expected rejected document")
	}

	if rejected.Status !=
		models.DriverDocumentStatusRejected {

		t.Fatalf(
			"expected status %s, got %s",
			models.DriverDocumentStatusRejected,
			rejected.Status,
		)
	}

	if rejected.VerifiedAt != nil {
		t.Fatalf(
			"expected nil verified_at, got %v",
			rejected.VerifiedAt,
		)
	}

	if rejected.VerifiedByUserID == nil ||
		*rejected.VerifiedByUserID != fixture.UserID {

		t.Fatalf(
			"expected reviewer user ID %s, got %v",
			fixture.UserID,
			rejected.VerifiedByUserID,
		)
	}

	if rejected.RejectionReason == nil ||
		*rejected.RejectionReason !=
			"document image is unreadable" {

		t.Fatalf(
			"unexpected rejection reason: %v",
			rejected.RejectionReason,
		)
	}

	persisted, err := documentRepo.GetByID(
		ctx,
		document.ID,
	)
	if err != nil {
		t.Fatalf(
			"load rejected document: %v",
			err,
		)
	}

	if persisted.Status !=
		models.DriverDocumentStatusRejected {

		t.Fatalf(
			"expected persisted status %s, got %s",
			models.DriverDocumentStatusRejected,
			persisted.Status,
		)
	}

	if persisted.VerifiedAt != nil {
		t.Fatalf(
			"expected persisted nil verified_at, got %v",
			persisted.VerifiedAt,
		)
	}

	if persisted.VerifiedByUserID == nil ||
		*persisted.VerifiedByUserID != fixture.UserID {

		t.Fatalf(
			"expected persisted reviewer %s, got %v",
			fixture.UserID,
			persisted.VerifiedByUserID,
		)
	}

	if persisted.RejectionReason == nil ||
		*persisted.RejectionReason !=
			"document image is unreadable" {

		t.Fatalf(
			"unexpected persisted rejection reason: %v",
			persisted.RejectionReason,
		)
	}

	if !rejected.UpdatedAt.Equal(
		persisted.UpdatedAt,
	) {
		t.Fatalf(
			"expected returned updated_at %v to match persisted %v",
			rejected.UpdatedAt,
			persisted.UpdatedAt,
		)
	}
}

func TestReviewDocumentRejectsSecondDecision(
	t *testing.T,
) {
	db := openDriverDocumentTestDB(t)
	t.Cleanup(db.Close)

	ctx := context.Background()

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

	t.Cleanup(func() {
		cleanupFixture(context.Background())
	})

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
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

	document := &models.DriverDocument{
		DriverID:     fixture.DriverID,
		DocumentType: models.DriverDocumentTypeTaxiDriverLicense,
		FileName:     "terminal-state-test.pdf",
		StorageKey: "drivers/" +
			fixture.DriverID +
			"/terminal-state-test.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 4096,
		ExpiresAt:     &expiry,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := documentRepo.Create(
		ctx,
		document,
	); err != nil {
		t.Fatalf(
			"create pending document: %v",
			err,
		)
	}

	verified, err := service.Verify(
		ctx,
		document.ID,
		fixture.UserID,
	)
	if err != nil {
		t.Fatalf(
			"first review decision: %v",
			err,
		)
	}

	if verified == nil {
		t.Fatal(
			"expected verified document",
		)
	}

	second, err := service.Reject(
		ctx,
		document.ID,
		fixture.UserID,
		"must not replace verified state",
	)

	if !errors.Is(
		err,
		ErrDocumentAlreadyReviewed,
	) {
		t.Fatalf(
			"expected ErrDocumentAlreadyReviewed, got %v",
			err,
		)
	}

	if second != nil {
		t.Fatalf(
			"expected nil document after second decision, got %+v",
			second,
		)
	}

	persisted, err := documentRepo.GetByID(
		ctx,
		document.ID,
	)
	if err != nil {
		t.Fatalf(
			"load terminal document: %v",
			err,
		)
	}

	if persisted.Status !=
		models.DriverDocumentStatusVerified {

		t.Fatalf(
			"expected VERIFIED state to remain unchanged, got %s",
			persisted.Status,
		)
	}

	if persisted.RejectionReason != nil {
		t.Fatalf(
			"expected rejection reason to remain nil, got %v",
			persisted.RejectionReason,
		)
	}
}

func TestReviewDocumentReturnsNotFound(
	t *testing.T,
) {
	db := openDriverDocumentTestDB(t)
	t.Cleanup(db.Close)

	ctx := context.Background()

	service := NewService(
		Dependencies{
			DB: db,
		},
	)

	document, err := service.Verify(
		ctx,
		uuid.NewString(),
		uuid.NewString(),
	)

	if !errors.Is(
		err,
		ErrDocumentNotFound,
	) {
		t.Fatalf(
			"expected ErrDocumentNotFound, got %v",
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

func TestConcurrentReviewAllowsExactlyOneDecision(
	t *testing.T,
) {
	db := openDriverDocumentTestDB(t)
	t.Cleanup(db.Close)

	ctx := context.Background()

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

	t.Cleanup(func() {
		cleanupFixture(context.Background())
	})

	t.Cleanup(func() {
		_, err := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
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

	document := &models.DriverDocument{
		DriverID:     fixture.DriverID,
		DocumentType: models.DriverDocumentTypeDrivingLicense,
		FileName:     "concurrent-review.pdf",
		StorageKey: "drivers/" +
			fixture.DriverID +
			"/concurrent-review.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 4096,
		ExpiresAt:     &expiry,
		Status:        models.DriverDocumentStatusPending,
	}

	if err := documentRepo.Create(
		ctx,
		document,
	); err != nil {
		t.Fatalf(
			"create pending document: %v",
			err,
		)
	}

	type reviewResult struct {
		action string
		err    error
	}

	start := make(chan struct{})
	results := make(chan reviewResult, 2)

	go func() {
		<-start

		_, err := service.Verify(
			context.Background(),
			document.ID,
			fixture.UserID,
		)

		results <- reviewResult{
			action: "verify",
			err:    err,
		}
	}()

	go func() {
		<-start

		_, err := service.Reject(
			context.Background(),
			document.ID,
			fixture.UserID,
			"concurrent rejection",
		)

		results <- reviewResult{
			action: "reject",
			err:    err,
		}
	}()

	close(start)

	first := <-results
	second := <-results

	successCount := 0
	alreadyReviewedCount := 0

	for _, result := range []reviewResult{
		first,
		second,
	} {
		switch {
		case result.err == nil:
			successCount++

		case errors.Is(
			result.err,
			ErrDocumentAlreadyReviewed,
		):
			alreadyReviewedCount++

		default:
			t.Fatalf(
				"unexpected %s review error: %v",
				result.action,
				result.err,
			)
		}
	}

	if successCount != 1 {
		t.Fatalf(
			"expected exactly one successful review, got %d",
			successCount,
		)
	}

	if alreadyReviewedCount != 1 {
		t.Fatalf(
			"expected exactly one already-reviewed result, got %d",
			alreadyReviewedCount,
		)
	}

	persisted, err := documentRepo.GetByID(
		ctx,
		document.ID,
	)
	if err != nil {
		t.Fatalf(
			"load concurrently reviewed document: %v",
			err,
		)
	}

	switch persisted.Status {
	case models.DriverDocumentStatusVerified:
		if persisted.VerifiedAt == nil {
			t.Fatal(
				"expected verified_at for VERIFIED result",
			)
		}

		if persisted.VerifiedByUserID == nil ||
			*persisted.VerifiedByUserID != fixture.UserID {

			t.Fatalf(
				"unexpected VERIFIED reviewer: %v",
				persisted.VerifiedByUserID,
			)
		}

		if persisted.RejectionReason != nil {
			t.Fatalf(
				"expected nil rejection reason for VERIFIED result, got %v",
				persisted.RejectionReason,
			)
		}

	case models.DriverDocumentStatusRejected:
		if persisted.VerifiedAt != nil {
			t.Fatalf(
				"expected nil verified_at for REJECTED result, got %v",
				persisted.VerifiedAt,
			)
		}

		if persisted.VerifiedByUserID == nil ||
			*persisted.VerifiedByUserID != fixture.UserID {

			t.Fatalf(
				"unexpected REJECTED reviewer: %v",
				persisted.VerifiedByUserID,
			)
		}

		if persisted.RejectionReason == nil ||
			*persisted.RejectionReason !=
				"concurrent rejection" {

			t.Fatalf(
				"unexpected rejection reason: %v",
				persisted.RejectionReason,
			)
		}

	default:
		t.Fatalf(
			"expected terminal document status, got %s",
			persisted.Status,
		)
	}
}
