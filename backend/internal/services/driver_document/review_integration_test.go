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

func TestVerifyDocumentWaitsForDriverAggregateLock(
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
		_, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
		)
		if cleanupErr != nil {
			t.Errorf(
				"cleanup driver documents: %v",
				cleanupErr,
			)
		}

		cleanupFixture(context.Background())
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
		FileName:     "aggregate-lock-review.pdf",
		StorageKey: "drivers/" +
			fixture.DriverID +
			"/aggregate-lock-review.pdf",
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

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf(
			"begin aggregate-lock transaction: %v",
			err,
		)
	}

	txFinished := false
	t.Cleanup(func() {
		if !txFinished {
			_ = tx.Rollback(context.Background())
		}
	})

	if _, err := tx.Exec(
		ctx,
		`
			SELECT id
			FROM drivers
			WHERE id = $1
			FOR UPDATE
		`,
		fixture.DriverID,
	); err != nil {
		t.Fatalf(
			"lock driver aggregate: %v",
			err,
		)
	}

	type verifyResult struct {
		document *models.DriverDocument
		err      error
	}

	started := make(chan struct{})
	result := make(chan verifyResult, 1)

	go func() {
		close(started)

		verified, verifyErr := service.Verify(
			context.Background(),
			document.ID,
			fixture.UserID,
		)

		result <- verifyResult{
			document: verified,
			err:      verifyErr,
		}
	}()

	<-started

	select {
	case early := <-result:
		t.Fatalf(
			"verify completed while driver aggregate lock was held: document=%+v err=%v",
			early.document,
			early.err,
		)

	case <-time.After(150 * time.Millisecond):
		// Expected: review resolved document ownership, then blocked while
		// acquiring the driver's aggregate serialization lock.
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf(
			"release driver aggregate lock: %v",
			err,
		)
	}
	txFinished = true

	select {
	case completed := <-result:
		if completed.err != nil {
			t.Fatalf(
				"verify after aggregate lock release: %v",
				completed.err,
			)
		}

		if completed.document == nil {
			t.Fatal(
				"expected verified document after aggregate lock release",
			)
		}

		if completed.document.Status !=
			models.DriverDocumentStatusVerified {

			t.Fatalf(
				"expected status %s, got %s",
				models.DriverDocumentStatusVerified,
				completed.document.Status,
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal(
			"verify did not complete after driver aggregate lock was released",
		)
	}
}

func TestRevokeDocumentPreservesVerificationAudit(
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
		FileName:     "revocation-test.pdf",
		StorageKey: "drivers/" +
			fixture.DriverID +
			"/revocation-test.pdf",
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
			"verify document before revocation: %v",
			err,
		)
	}

	if verified == nil ||
		verified.VerifiedAt == nil ||
		verified.VerifiedByUserID == nil {

		t.Fatalf(
			"expected complete verification audit, got %+v",
			verified,
		)
	}

	persistedVerified, err := documentRepo.GetByID(
		ctx,
		document.ID,
	)
	if err != nil {
		t.Fatalf(
			"load persisted verification audit: %v",
			err,
		)
	}

	if persistedVerified.VerifiedAt == nil ||
		persistedVerified.VerifiedByUserID == nil {

		t.Fatalf(
			"expected persisted verification audit, got %+v",
			persistedVerified,
		)
	}

	originalVerifiedAt :=
		*persistedVerified.VerifiedAt
	originalVerifiedByUserID :=
		*persistedVerified.VerifiedByUserID

	const reason = "credential withdrawn by issuing authority"

	beforeRevocation := time.Now().UTC()

	revoked, err := service.Revoke(
		ctx,
		document.ID,
		fixture.UserID,
		reason,
	)
	if err != nil {
		t.Fatalf(
			"revoke verified document: %v",
			err,
		)
	}

	if revoked == nil {
		t.Fatal("expected revoked document")
	}

	if revoked.Status !=
		models.DriverDocumentStatusRevoked {

		t.Fatalf(
			"expected status %s, got %s",
			models.DriverDocumentStatusRevoked,
			revoked.Status,
		)
	}

	if revoked.VerifiedAt == nil ||
		!revoked.VerifiedAt.Equal(
			originalVerifiedAt,
		) {

		t.Fatalf(
			"verification timestamp changed during revocation: got %v want %v",
			revoked.VerifiedAt,
			originalVerifiedAt,
		)
	}

	if revoked.VerifiedByUserID == nil ||
		*revoked.VerifiedByUserID !=
			originalVerifiedByUserID {

		t.Fatalf(
			"verification reviewer changed during revocation: got %v want %s",
			revoked.VerifiedByUserID,
			originalVerifiedByUserID,
		)
	}

	if revoked.RejectionReason != nil {
		t.Fatalf(
			"expected nil rejection reason, got %v",
			revoked.RejectionReason,
		)
	}

	if revoked.RevokedAt == nil {
		t.Fatal("expected revoked_at")
	}

	if revoked.RevokedAt.Before(beforeRevocation) {
		t.Fatalf(
			"expected revoked_at >= %v, got %v",
			beforeRevocation,
			*revoked.RevokedAt,
		)
	}

	if revoked.RevokedByUserID == nil ||
		*revoked.RevokedByUserID != fixture.UserID {

		t.Fatalf(
			"expected revocation reviewer %s, got %v",
			fixture.UserID,
			revoked.RevokedByUserID,
		)
	}

	if revoked.RevocationReason == nil ||
		*revoked.RevocationReason != reason {

		t.Fatalf(
			"expected revocation reason %q, got %v",
			reason,
			revoked.RevocationReason,
		)
	}

	persisted, err := documentRepo.GetByID(
		ctx,
		document.ID,
	)
	if err != nil {
		t.Fatalf(
			"load revoked document: %v",
			err,
		)
	}

	if persisted.Status !=
		models.DriverDocumentStatusRevoked {

		t.Fatalf(
			"expected persisted status %s, got %s",
			models.DriverDocumentStatusRevoked,
			persisted.Status,
		)
	}

	if persisted.VerifiedAt == nil ||
		!persisted.VerifiedAt.Equal(
			originalVerifiedAt,
		) {

		t.Fatalf(
			"persisted verification timestamp changed: got %v want %v",
			persisted.VerifiedAt,
			originalVerifiedAt,
		)
	}

	if persisted.VerifiedByUserID == nil ||
		*persisted.VerifiedByUserID !=
			originalVerifiedByUserID {

		t.Fatalf(
			"persisted verification reviewer changed: got %v want %s",
			persisted.VerifiedByUserID,
			originalVerifiedByUserID,
		)
	}

	if persisted.RevokedAt == nil {
		t.Fatal("expected persisted revoked_at")
	}

	if persisted.RevokedByUserID == nil ||
		*persisted.RevokedByUserID != fixture.UserID {

		t.Fatalf(
			"expected persisted revocation reviewer %s, got %v",
			fixture.UserID,
			persisted.RevokedByUserID,
		)
	}

	if persisted.RevocationReason == nil ||
		*persisted.RevocationReason != reason {

		t.Fatalf(
			"expected persisted revocation reason %q, got %v",
			reason,
			persisted.RevocationReason,
		)
	}

	if !revoked.UpdatedAt.Equal(
		persisted.UpdatedAt,
	) {
		t.Fatalf(
			"expected returned updated_at %v to match persisted %v",
			revoked.UpdatedAt,
			persisted.UpdatedAt,
		)
	}
}

func TestRevokeDocumentRejectsNonVerifiedStates(
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

	tests := []struct {
		name         string
		documentType string
		prepare      func(
			*models.DriverDocument,
		) error
		wantStatus string
	}{
		{
			name:         "pending",
			documentType: models.DriverDocumentTypeDrivingLicense,
			prepare: func(
				document *models.DriverDocument,
			) error {
				return nil
			},
			wantStatus: models.DriverDocumentStatusPending,
		},
		{
			name:         "rejected",
			documentType: models.DriverDocumentTypeTaxiDriverLicense,
			prepare: func(
				document *models.DriverDocument,
			) error {
				_, err := service.Reject(
					ctx,
					document.ID,
					fixture.UserID,
					"submission rejected",
				)
				return err
			},
			wantStatus: models.DriverDocumentStatusRejected,
		},
		{
			name:         "already revoked",
			documentType: models.DriverDocumentTypeDrivingLicense,
			prepare: func(
				document *models.DriverDocument,
			) error {
				if _, err := service.Verify(
					ctx,
					document.ID,
					fixture.UserID,
				); err != nil {
					return err
				}

				_, err := service.Revoke(
					ctx,
					document.ID,
					fixture.UserID,
					"initial revocation",
				)
				return err
			},
			wantStatus: models.DriverDocumentStatusRevoked,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				// The active-document uniqueness constraint permits only one
				// document of a type per driver, so clear the previous
				// subtest's document before creating the next one.
				if _, err := db.Exec(
					ctx,
					`
						DELETE FROM driver_documents
						WHERE driver_id = $1
					`,
					fixture.DriverID,
				); err != nil {
					t.Fatalf(
						"clear previous document: %v",
						err,
					)
				}

				expiry := time.Now().
					UTC().
					AddDate(1, 0, 0)

				document := &models.DriverDocument{
					DriverID:     fixture.DriverID,
					DocumentType: test.documentType,
					FileName: test.name +
						"-revocation-state.pdf",
					StorageKey: "drivers/" +
						fixture.DriverID +
						"/" +
						test.name +
						"-revocation-state.pdf",
					ContentType:   "application/pdf",
					FileSizeBytes: 4096,
					ExpiresAt:     &expiry,
					Status: models.
						DriverDocumentStatusPending,
				}

				if err := documentRepo.Create(
					ctx,
					document,
				); err != nil {
					t.Fatalf(
						"create document: %v",
						err,
					)
				}

				if err := test.prepare(
					document,
				); err != nil {
					t.Fatalf(
						"prepare %s state: %v",
						test.name,
						err,
					)
				}

				before, err :=
					documentRepo.GetByID(
						ctx,
						document.ID,
					)
				if err != nil {
					t.Fatalf(
						"load document before invalid revocation: %v",
						err,
					)
				}

				revoked, err := service.Revoke(
					ctx,
					document.ID,
					fixture.UserID,
					"must not change state",
				)

				if !errors.Is(
					err,
					ErrDocumentNotRevocable,
				) {
					t.Fatalf(
						"expected ErrDocumentNotRevocable, got %v",
						err,
					)
				}

				if revoked != nil {
					t.Fatalf(
						"expected nil document, got %+v",
						revoked,
					)
				}

				after, err :=
					documentRepo.GetByID(
						ctx,
						document.ID,
					)
				if err != nil {
					t.Fatalf(
						"load document after invalid revocation: %v",
						err,
					)
				}

				if after.Status != test.wantStatus {
					t.Fatalf(
						"status changed: got %s want %s",
						after.Status,
						test.wantStatus,
					)
				}

				if !after.UpdatedAt.Equal(
					before.UpdatedAt,
				) {
					t.Fatalf(
						"invalid revocation mutated updated_at: before %v after %v",
						before.UpdatedAt,
						after.UpdatedAt,
					)
				}

				if after.Status !=
					models.DriverDocumentStatusRevoked {

					if after.RevokedAt != nil ||
						after.RevokedByUserID != nil ||
						after.RevocationReason != nil {

						t.Fatalf(
							"invalid revocation wrote revocation audit: %+v",
							after,
						)
					}
				}
			},
		)
	}
}

func TestRevokeDocumentWaitsForDriverAggregateLock(
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
		_, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM driver_documents
				WHERE driver_id = $1
			`,
			fixture.DriverID,
		)
		if cleanupErr != nil {
			t.Errorf(
				"cleanup driver documents: %v",
				cleanupErr,
			)
		}

		cleanupFixture(context.Background())
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
		FileName:     "aggregate-lock-revocation.pdf",
		StorageKey: "drivers/" +
			fixture.DriverID +
			"/aggregate-lock-revocation.pdf",
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

	if _, err := service.Verify(
		ctx,
		document.ID,
		fixture.UserID,
	); err != nil {
		t.Fatalf(
			"verify document before lock test: %v",
			err,
		)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf(
			"begin aggregate-lock transaction: %v",
			err,
		)
	}

	txFinished := false
	t.Cleanup(func() {
		if !txFinished {
			_ = tx.Rollback(context.Background())
		}
	})

	if _, err := tx.Exec(
		ctx,
		`
			SELECT id
			FROM drivers
			WHERE id = $1
			FOR UPDATE
		`,
		fixture.DriverID,
	); err != nil {
		t.Fatalf(
			"lock driver aggregate: %v",
			err,
		)
	}

	type revokeResult struct {
		document *models.DriverDocument
		err      error
	}

	started := make(chan struct{})
	result := make(chan revokeResult, 1)

	go func() {
		close(started)

		revoked, revokeErr := service.Revoke(
			context.Background(),
			document.ID,
			fixture.UserID,
			"credential withdrawn",
		)

		result <- revokeResult{
			document: revoked,
			err:      revokeErr,
		}
	}()

	<-started

	select {
	case early := <-result:
		t.Fatalf(
			"revoke completed while driver aggregate lock was held: document=%+v err=%v",
			early.document,
			early.err,
		)

	case <-time.After(150 * time.Millisecond):
		// Expected: revocation resolved document ownership, then blocked while
		// acquiring the driver's aggregate serialization lock.
	}

	// While the aggregate lock is held, the document must still be VERIFIED.
	// This proves revocation has not mutated eligibility state before acquiring
	// the canonical driver lock.
	whileBlocked, err := documentRepo.GetByID(
		ctx,
		document.ID,
	)
	if err != nil {
		t.Fatalf(
			"load document while revocation is blocked: %v",
			err,
		)
	}

	if whileBlocked.Status !=
		models.DriverDocumentStatusVerified {

		t.Fatalf(
			"document mutated before aggregate lock acquisition: got %s want %s",
			whileBlocked.Status,
			models.DriverDocumentStatusVerified,
		)
	}

	if whileBlocked.RevokedAt != nil ||
		whileBlocked.RevokedByUserID != nil ||
		whileBlocked.RevocationReason != nil {

		t.Fatalf(
			"revocation audit written before aggregate lock acquisition: %+v",
			whileBlocked,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf(
			"release driver aggregate lock: %v",
			err,
		)
	}
	txFinished = true

	select {
	case completed := <-result:
		if completed.err != nil {
			t.Fatalf(
				"revoke after aggregate lock release: %v",
				completed.err,
			)
		}

		if completed.document == nil {
			t.Fatal(
				"expected revoked document after aggregate lock release",
			)
		}

		if completed.document.Status !=
			models.DriverDocumentStatusRevoked {

			t.Fatalf(
				"expected status %s, got %s",
				models.DriverDocumentStatusRevoked,
				completed.document.Status,
			)
		}

		if completed.document.RevokedAt == nil {
			t.Fatal(
				"expected revoked_at after aggregate lock release",
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal(
			"revoke did not complete after driver aggregate lock was released",
		)
	}
}
