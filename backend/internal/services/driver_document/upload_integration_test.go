package driver_document

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	documentstorage "github.com/JCKFinland/connect/backend/internal/storage/document"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestUploadForUserStorageFailureDoesNotPersistMetadata(
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

	defer cleanupDriverDocumentUploadFixture(
		t,
		db,
		fixture.DriverID,
		cleanupFixture,
	)

	putErr := errors.New(
		"controlled storage put failure",
	)

	storage := &driverDocumentStorageStub{
		putErr: putErr,
	}

	service := newDriverDocumentUploadTestService(
		db,
		storage,
	)

	expiry := time.Now().
		UTC().
		AddDate(1, 0, 0)

	document, err := service.UploadForUser(
		ctx,
		fixture.UserID,
		validUploadDocumentRequest(
			&expiry,
		),
	)

	if !errors.Is(err, putErr) {
		t.Fatalf(
			"expected storage put failure, got %v",
			err,
		)
	}

	if document != nil {
		t.Fatalf(
			"expected nil document, got %+v",
			document,
		)
	}

	if storage.putCalls != 1 {
		t.Fatalf(
			"expected 1 Put call, got %d",
			storage.putCalls,
		)
	}

	if storage.deleteCalls != 0 {
		t.Fatalf(
			"expected no Delete call after failed Put, got %d",
			storage.deleteCalls,
		)
	}

	documentRepo :=
		postgresrepo.NewDriverDocumentRepository(db)

	_, err = documentRepo.GetByDriverAndType(
		ctx,
		fixture.DriverID,
		models.DriverDocumentTypeDrivingLicense,
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected no persisted metadata, got %v",
			err,
		)
	}
}

func TestUploadForUserSuccessfulStorageAndPersistenceRetainsObject(
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

	defer cleanupDriverDocumentUploadFixture(
		t,
		db,
		fixture.DriverID,
		cleanupFixture,
	)

	storage := &driverDocumentStorageStub{}

	service := newDriverDocumentUploadTestService(
		db,
		storage,
	)

	expiry := time.Now().
		UTC().
		AddDate(1, 0, 0)

	document, err := service.UploadForUser(
		ctx,
		fixture.UserID,
		validUploadDocumentRequest(
			&expiry,
		),
	)
	if err != nil {
		t.Fatalf(
			"upload driver document: %v",
			err,
		)
	}

	if document == nil {
		t.Fatal(
			"expected persisted document",
		)
	}

	if document.DriverID != fixture.DriverID {
		t.Fatalf(
			"expected driver ID %s, got %s",
			fixture.DriverID,
			document.DriverID,
		)
	}

	if storage.putCalls != 1 {
		t.Fatalf(
			"expected 1 Put call, got %d",
			storage.putCalls,
		)
	}

	if storage.deleteCalls != 0 {
		t.Fatalf(
			"successful upload must retain stored object; Delete calls: %d",
			storage.deleteCalls,
		)
	}

	if storage.lastPut.Key == "" {
		t.Fatal(
			"expected server-generated storage key",
		)
	}

	expectedPrefix :=
		"drivers/" +
			fixture.DriverID +
			"/driving_license/"

	if !strings.HasPrefix(
		storage.lastPut.Key,
		expectedPrefix,
	) {
		t.Fatalf(
			"unexpected storage key %q",
			storage.lastPut.Key,
		)
	}

	if document.StorageKey != storage.lastPut.Key {
		t.Fatalf(
			"persisted storage key %q does not match stored object %q",
			document.StorageKey,
			storage.lastPut.Key,
		)
	}

	if string(storage.lastPutBody) !=
		"%PDF-1.7\\nCONNECT test regulatory document" {
		t.Fatalf(
			"unexpected stored body %q",
			storage.lastPutBody,
		)
	}

	documentRepo :=
		postgresrepo.NewDriverDocumentRepository(db)

	persisted, err :=
		documentRepo.GetByDriverAndType(
			ctx,
			fixture.DriverID,
			models.DriverDocumentTypeDrivingLicense,
		)
	if err != nil {
		t.Fatalf(
			"load persisted document: %v",
			err,
		)
	}

	if persisted.ID != document.ID {
		t.Fatalf(
			"expected persisted document %s, got %s",
			document.ID,
			persisted.ID,
		)
	}

	if persisted.StorageKey !=
		storage.lastPut.Key {
		t.Fatalf(
			"expected persisted storage key %q, got %q",
			storage.lastPut.Key,
			persisted.StorageKey,
		)
	}
}

func TestUploadForUserPersistenceFailureDeletesStoredObject(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)

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

	defer cleanupDriverDocumentUploadFixture(
		t,
		db,
		fixture.DriverID,
		cleanupFixture,
	)

	storage := &driverDocumentStorageStub{
		afterPut: cancel,
	}

	service := newDriverDocumentUploadTestService(
		db,
		storage,
	)

	expiry := time.Now().
		UTC().
		AddDate(1, 0, 0)

	document, err := service.UploadForUser(
		ctx,
		fixture.UserID,
		validUploadDocumentRequest(
			&expiry,
		),
	)

	if err == nil {
		t.Fatal(
			"expected persistence failure after context cancellation",
		)
	}

	if document != nil {
		t.Fatalf(
			"expected nil document, got %+v",
			document,
		)
	}

	if storage.putCalls != 1 {
		t.Fatalf(
			"expected 1 Put call, got %d",
			storage.putCalls,
		)
	}

	if storage.deleteCalls != 1 {
		t.Fatalf(
			"expected compensating Delete call, got %d",
			storage.deleteCalls,
		)
	}

	if storage.lastDeleteKey !=
		storage.lastPut.Key {
		t.Fatalf(
			"cleanup key %q does not match stored key %q",
			storage.lastDeleteKey,
			storage.lastPut.Key,
		)
	}

	if storage.deleteContextErr != nil {
		t.Fatalf(
			"cleanup context must survive request cancellation, got %v",
			storage.deleteContextErr,
		)
	}

	documentRepo :=
		postgresrepo.NewDriverDocumentRepository(db)

	lookupCtx := context.Background()

	_, lookupErr :=
		documentRepo.GetByDriverAndType(
			lookupCtx,
			fixture.DriverID,
			models.DriverDocumentTypeDrivingLicense,
		)
	if !errors.Is(
		lookupErr,
		repository.ErrNotFound,
	) {
		t.Fatalf(
			"expected no persisted metadata after failed submission, got %v",
			lookupErr,
		)
	}
}

func TestUploadForUserPreservesPersistenceAndCleanupFailures(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)

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

	defer cleanupDriverDocumentUploadFixture(
		t,
		db,
		fixture.DriverID,
		cleanupFixture,
	)

	cleanupErr := errors.New(
		"controlled cleanup failure",
	)

	storage := &driverDocumentStorageStub{
		afterPut:  cancel,
		deleteErr: cleanupErr,
	}

	service := newDriverDocumentUploadTestService(
		db,
		storage,
	)

	expiry := time.Now().
		UTC().
		AddDate(1, 0, 0)

	document, err := service.UploadForUser(
		ctx,
		fixture.UserID,
		validUploadDocumentRequest(
			&expiry,
		),
	)

	if err == nil {
		t.Fatal(
			"expected joined persistence and cleanup failure",
		)
	}

	if document != nil {
		t.Fatalf(
			"expected nil document, got %+v",
			document,
		)
	}

	if !errors.Is(
		err,
		cleanupErr,
	) {
		t.Fatalf(
			"expected cleanup failure to remain discoverable, got %v",
			err,
		)
	}

	if storage.deleteCalls != 1 {
		t.Fatalf(
			"expected 1 compensating Delete call, got %d",
			storage.deleteCalls,
		)
	}

	if storage.deleteContextErr != nil {
		t.Fatalf(
			"cleanup context must survive request cancellation, got %v",
			storage.deleteContextErr,
		)
	}

	if !strings.Contains(
		err.Error(),
		"cleanup stored driver document after persistence failure",
	) {
		t.Fatalf(
			"expected cleanup failure context, got %v",
			err,
		)
	}

	// errors.Join must preserve the original persistence failure as well as
	// the cleanup failure. The exact pgx error text is intentionally not
	// asserted because it is an implementation detail.
	if errors.Is(
		err,
		cleanupErr,
	) && err.Error() ==
		cleanupErr.Error() {
		t.Fatalf(
			"cleanup failure replaced the persistence failure: %v",
			err,
		)
	}
}

type driverDocumentStorageStub struct {
	putErr    error
	deleteErr error
	afterPut  func()

	putCalls    int
	deleteCalls int

	lastPut       documentstorage.PutRequest
	lastPutBody   []byte
	lastDeleteKey string

	deleteContextErr error
}

func (s *driverDocumentStorageStub) Put(
	ctx context.Context,
	req documentstorage.PutRequest,
) error {
	s.putCalls++

	s.lastPut = documentstorage.PutRequest{
		Key: req.Key,
	}

	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf(
				"read test upload body: %w",
				err,
			)
		}

		s.lastPutBody = body
	}

	if s.putErr != nil {
		return s.putErr
	}

	if s.afterPut != nil {
		s.afterPut()
	}

	return nil
}

func (s *driverDocumentStorageStub) Delete(
	ctx context.Context,
	key string,
) error {
	s.deleteCalls++
	s.lastDeleteKey = key
	s.deleteContextErr = ctx.Err()

	return s.deleteErr
}

func validUploadDocumentRequest(
	expiry *time.Time,
) UploadDocumentRequest {
	return UploadDocumentRequest{
		DocumentType: models.DriverDocumentTypeDrivingLicense,
		FileName:     "driving-license-client-name.exe",
		ExpiresAt:    expiry,
		Body: bytes.NewBufferString(
			"%PDF-1.7\\nCONNECT test regulatory document",
		),
	}
}

func newDriverDocumentUploadTestService(
	db *pgxpool.Pool,
	storage documentstorage.Storage,
) *Service {
	return NewService(
		Dependencies{
			DB: db,
			Documents: postgresrepo.NewDriverDocumentRepository(
				db,
			),
			Drivers: postgresrepo.NewDriverRepository(
				db,
			),
			Storage:        storage,
			UploadMaxBytes: 10 * 1024 * 1024,
		},
	)
}

func cleanupDriverDocumentUploadFixture(
	t *testing.T,
	db *pgxpool.Pool,
	driverID string,
	cleanupFixture func(context.Context) error,
) {
	t.Helper()

	cleanupCtx := context.Background()

	if _, err := db.Exec(
		cleanupCtx,
		`
			DELETE FROM driver_documents
			WHERE driver_id = $1
		`,
		driverID,
	); err != nil {
		t.Logf(
			"cleanup driver documents: %v",
			err,
		)
	}

	if err := cleanupFixture(
		cleanupCtx,
	); err != nil {
		t.Logf(
			"cleanup driver fixture: %v",
			err,
		)
	}
}
