package driver_document

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	documentstorage "github.com/JCKFinland/connect/backend/internal/storage/document"
)

type openDocumentStorageStub struct {
	body []byte
	err  error

	openCalls int
	openKey   string
}

func (s *openDocumentStorageStub) Put(
	context.Context,
	documentstorage.PutRequest,
) error {
	return nil
}

func (s *openDocumentStorageStub) Open(
	_ context.Context,
	key string,
) (io.ReadCloser, error) {
	s.openCalls++
	s.openKey = key

	if s.err != nil {
		return nil, s.err
	}

	return io.NopCloser(
		bytes.NewReader(s.body),
	), nil
}

func (s *openDocumentStorageStub) Delete(
	context.Context,
	string,
) error {
	return nil
}

func TestOpenDocumentVerifiesOwnershipBeforeOpeningStorage(
	t *testing.T,
) {
	const storageKey = "drivers/driver-123/driving_license/document.pdf"

	repo := &documentRepositoryStub{
		getDocument: &models.DriverDocument{
			BaseModel: models.BaseModel{
				ID: "document-123",
			},
			DriverID:      "driver-123",
			DocumentType:  models.DriverDocumentTypeDrivingLicense,
			FileName:      "license.pdf",
			StorageKey:    storageKey,
			ContentType:   "application/pdf",
			FileSizeBytes: 15,
			Status:        models.DriverDocumentStatusPending,
		},
	}

	storage := &openDocumentStorageStub{
		body: []byte("%PDF-1.7\nCONNECT"),
	}

	service := NewService(
		Dependencies{
			Documents: repo,
			Storage:   storage,
		},
	)

	opened, err := service.Open(
		context.Background(),
		"driver-123",
		"document-123",
	)
	if err != nil {
		t.Fatalf("open document: %v", err)
	}
	if opened == nil {
		t.Fatal("expected opened document")
	}
	if opened.Body == nil {
		t.Fatal("expected document body")
	}
	defer opened.Body.Close()

	if opened.Document == nil {
		t.Fatal("expected document metadata")
	}
	if opened.Document.ID != "document-123" {
		t.Fatalf(
			"document ID mismatch: got %q",
			opened.Document.ID,
		)
	}

	if storage.openCalls != 1 {
		t.Fatalf(
			"expected one storage open, got %d",
			storage.openCalls,
		)
	}
	if storage.openKey != storageKey {
		t.Fatalf(
			"storage key mismatch: got %q want %q",
			storage.openKey,
			storageKey,
		)
	}

	body, err := io.ReadAll(opened.Body)
	if err != nil {
		t.Fatalf("read opened document: %v", err)
	}
	if !bytes.Equal(
		body,
		[]byte("%PDF-1.7\nCONNECT"),
	) {
		t.Fatalf("unexpected document body: %q", body)
	}
}

func TestOpenDocumentDoesNotOpenStorageForWrongDriver(
	t *testing.T,
) {
	repo := &documentRepositoryStub{
		getDocument: &models.DriverDocument{
			BaseModel: models.BaseModel{
				ID: "document-123",
			},
			DriverID:   "driver-other",
			StorageKey: "drivers/driver-other/file.pdf",
		},
	}

	storage := &openDocumentStorageStub{}

	service := NewService(
		Dependencies{
			Documents: repo,
			Storage:   storage,
		},
	)

	opened, err := service.Open(
		context.Background(),
		"driver-123",
		"document-123",
	)

	if !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf(
			"expected ErrDocumentNotFound, got %v",
			err,
		)
	}
	if opened != nil {
		t.Fatal("expected no opened document")
	}
	if storage.openCalls != 0 {
		t.Fatalf(
			"storage must not be opened before ownership succeeds; got %d calls",
			storage.openCalls,
		)
	}
}

func TestOpenDocumentMapsMissingBinaryToDocumentNotFound(
	t *testing.T,
) {
	repo := &documentRepositoryStub{
		getDocument: &models.DriverDocument{
			BaseModel: models.BaseModel{
				ID: "document-123",
			},
			DriverID:   "driver-123",
			StorageKey: "drivers/driver-123/file.pdf",
		},
	}

	storage := &openDocumentStorageStub{
		err: documentstorage.ErrNotFound,
	}

	service := NewService(
		Dependencies{
			Documents: repo,
			Storage:   storage,
		},
	)

	opened, err := service.Open(
		context.Background(),
		"driver-123",
		"document-123",
	)

	if !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf(
			"expected ErrDocumentNotFound, got %v",
			err,
		)
	}
	if opened != nil {
		t.Fatal("expected no opened document")
	}
}

func TestOpenForUserResolvesDriverBeforeOpeningDocument(
	t *testing.T,
) {
	const storageKey = "drivers/driver-123/driving_license/document.pdf"

	driverRepo := &selfServiceDriverRepository{
		driver: &models.Driver{
			BaseModel: models.BaseModel{
				ID: "driver-123",
			},
			UserID: "user-123",
		},
	}

	documentRepo := &documentRepositoryStub{
		getDocument: &models.DriverDocument{
			BaseModel: models.BaseModel{
				ID: "document-123",
			},
			DriverID:   "driver-123",
			StorageKey: storageKey,
		},
	}

	storage := &openDocumentStorageStub{
		body: []byte("%PDF-1.7\nCONNECT"),
	}

	service := NewService(
		Dependencies{
			Documents: documentRepo,
			Drivers:   driverRepo,
			Storage:   storage,
		},
	)

	opened, err := service.OpenForUser(
		context.Background(),
		" user-123 ",
		"document-123",
	)
	if err != nil {
		t.Fatalf("open document for user: %v", err)
	}
	if opened == nil {
		t.Fatal("expected opened document")
	}
	defer opened.Body.Close()

	if !driverRepo.getByUserIDCalled {
		t.Fatal("expected authenticated user lookup")
	}
	if driverRepo.userID != "user-123" {
		t.Fatalf(
			"user ID mismatch: got %q want %q",
			driverRepo.userID,
			"user-123",
		)
	}
	if storage.openCalls != 1 {
		t.Fatalf(
			"expected one storage open, got %d",
			storage.openCalls,
		)
	}
}
