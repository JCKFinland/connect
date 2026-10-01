package driver_document

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type documentRepositoryStub struct {
	getDocument *models.DriverDocument
	getErr      error

	listDocuments []models.DriverDocument
	listErr       error

	gotGetID      string
	gotListDriver string
}

func (r *documentRepositoryStub) Create(
	ctx context.Context,
	document *models.DriverDocument,
) error {
	return nil
}

func (r *documentRepositoryStub) GetByID(
	ctx context.Context,
	id string,
) (*models.DriverDocument, error) {
	r.gotGetID = id

	if r.getErr != nil {
		return nil, r.getErr
	}

	return r.getDocument, nil
}

func (r *documentRepositoryStub) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.DriverDocument, error) {
	return r.GetByID(
		ctx,
		id,
	)
}

func (r *documentRepositoryStub) UpdateReviewState(
	ctx context.Context,
	id string,
	status string,
	verifiedAt *time.Time,
	verifiedByUserID string,
	rejectionReason *string,
) (time.Time, error) {
	return time.Now().UTC(), nil
}

func (r *documentRepositoryStub) UpdateRevocationState(
	ctx context.Context,
	id string,
	revokedAt time.Time,
	revokedByUserID string,
	revocationReason string,
) (time.Time, error) {
	return time.Now().UTC(), nil
}

func (r *documentRepositoryStub) GetByDriverAndType(
	ctx context.Context,
	driverID string,
	documentType string,
) (*models.DriverDocument, error) {
	return nil, repository.ErrNotFound
}

func (r *documentRepositoryStub) ListByDriver(
	ctx context.Context,
	driverID string,
) ([]models.DriverDocument, error) {
	r.gotListDriver = driverID

	if r.listErr != nil {
		return nil, r.listErr
	}

	return r.listDocuments, nil
}

func (r *documentRepositoryStub) SoftDelete(
	ctx context.Context,
	id string,
) error {
	return nil
}

func TestGetDocumentReturnsOwnedDocument(t *testing.T) {
	repo := &documentRepositoryStub{
		getDocument: &models.DriverDocument{
			BaseModel: models.BaseModel{
				ID: "document-123",
			},
			DriverID:     "driver-123",
			DocumentType: models.DriverDocumentTypeDrivingLicense,
			Status:       models.DriverDocumentStatusPending,
		},
	}

	service := NewService(
		Dependencies{
			Documents: repo,
		},
	)

	document, err := service.Get(
		context.Background(),
		" driver-123 ",
		" document-123 ",
	)
	if err != nil {
		t.Fatalf(
			"get owned document: %v",
			err,
		)
	}

	if document == nil {
		t.Fatal("expected document")
	}

	if document.ID != "document-123" {
		t.Fatalf(
			"expected document ID %q, got %q",
			"document-123",
			document.ID,
		)
	}

	if repo.gotGetID != "document-123" {
		t.Fatalf(
			"expected trimmed document ID, got %q",
			repo.gotGetID,
		)
	}
}

func TestGetDocumentReturnsNotFoundForRepositoryMiss(
	t *testing.T,
) {
	repo := &documentRepositoryStub{
		getErr: repository.ErrNotFound,
	}

	service := NewService(
		Dependencies{
			Documents: repo,
		},
	)

	document, err := service.Get(
		context.Background(),
		"driver-123",
		"missing-document",
	)

	if !errors.Is(err, ErrDocumentNotFound) {
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

func TestGetDocumentHidesDocumentOwnedByAnotherDriver(
	t *testing.T,
) {
	repo := &documentRepositoryStub{
		getDocument: &models.DriverDocument{
			BaseModel: models.BaseModel{
				ID: "document-123",
			},
			DriverID: "driver-other",
		},
	}

	service := NewService(
		Dependencies{
			Documents: repo,
		},
	)

	document, err := service.Get(
		context.Background(),
		"driver-123",
		"document-123",
	)

	if !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf(
			"expected ErrDocumentNotFound for cross-driver access, got %v",
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

func TestGetDocumentRejectsEmptyIdentifiers(t *testing.T) {
	service := NewService(
		Dependencies{
			Documents: &documentRepositoryStub{},
		},
	)

	tests := []struct {
		name       string
		driverID   string
		documentID string
	}{
		{
			name:       "missing driver ID",
			driverID:   "   ",
			documentID: "document-123",
		},
		{
			name:       "missing document ID",
			driverID:   "driver-123",
			documentID: "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document, err := service.Get(
				context.Background(),
				tt.driverID,
				tt.documentID,
			)

			if !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf(
					"expected ErrInvalidDocument, got %v",
					err,
				)
			}

			if document != nil {
				t.Fatalf(
					"expected nil document, got %+v",
					document,
				)
			}
		})
	}
}

func TestGetDocumentWrapsRepositoryError(t *testing.T) {
	repositoryFailure := errors.New(
		"repository failure",
	)

	repo := &documentRepositoryStub{
		getErr: repositoryFailure,
	}

	service := NewService(
		Dependencies{
			Documents: repo,
		},
	)

	document, err := service.Get(
		context.Background(),
		"driver-123",
		"document-123",
	)

	if !errors.Is(err, repositoryFailure) {
		t.Fatalf(
			"expected wrapped repository error, got %v",
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

func TestListDocumentsReturnsDriverDocuments(t *testing.T) {
	repo := &documentRepositoryStub{
		listDocuments: []models.DriverDocument{
			{
				BaseModel: models.BaseModel{
					ID: "document-1",
				},
				DriverID: "driver-123",
			},
			{
				BaseModel: models.BaseModel{
					ID: "document-2",
				},
				DriverID: "driver-123",
			},
		},
	}

	service := NewService(
		Dependencies{
			Documents: repo,
		},
	)

	documents, err := service.List(
		context.Background(),
		" driver-123 ",
	)
	if err != nil {
		t.Fatalf(
			"list documents: %v",
			err,
		)
	}

	if repo.gotListDriver != "driver-123" {
		t.Fatalf(
			"expected trimmed driver ID, got %q",
			repo.gotListDriver,
		)
	}

	if len(documents) != 2 {
		t.Fatalf(
			"expected 2 documents, got %d",
			len(documents),
		)
	}

	for _, document := range documents {
		if document.DriverID != "driver-123" {
			t.Fatalf(
				"unexpected driver document: %+v",
				document,
			)
		}
	}
}

func TestListDocumentsReturnsNonNilEmptySlice(t *testing.T) {
	repo := &documentRepositoryStub{
		listDocuments: nil,
	}

	service := NewService(
		Dependencies{
			Documents: repo,
		},
	)

	documents, err := service.List(
		context.Background(),
		"driver-123",
	)
	if err != nil {
		t.Fatalf(
			"list empty documents: %v",
			err,
		)
	}

	if documents == nil {
		t.Fatal("expected non-nil empty document slice")
	}

	if len(documents) != 0 {
		t.Fatalf(
			"expected zero documents, got %d",
			len(documents),
		)
	}
}

func TestListDocumentsRejectsEmptyDriverID(t *testing.T) {
	service := NewService(
		Dependencies{
			Documents: &documentRepositoryStub{},
		},
	)

	documents, err := service.List(
		context.Background(),
		"   ",
	)

	if !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf(
			"expected ErrInvalidDocument, got %v",
			err,
		)
	}

	if documents != nil {
		t.Fatalf(
			"expected nil documents, got %+v",
			documents,
		)
	}
}

func TestListDocumentsWrapsRepositoryError(t *testing.T) {
	repositoryFailure := errors.New(
		"repository failure",
	)

	repo := &documentRepositoryStub{
		listErr: repositoryFailure,
	}

	service := NewService(
		Dependencies{
			Documents: repo,
		},
	)

	documents, err := service.List(
		context.Background(),
		"driver-123",
	)

	if !errors.Is(err, repositoryFailure) {
		t.Fatalf(
			"expected wrapped repository error, got %v",
			err,
		)
	}

	if documents != nil {
		t.Fatalf(
			"expected nil documents, got %+v",
			documents,
		)
	}
}
