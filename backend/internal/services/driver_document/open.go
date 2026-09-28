package driver_document

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/JCKFinland/connect/backend/internal/models"
	documentstorage "github.com/JCKFinland/connect/backend/internal/storage/document"
)

// OpenDocument contains an authorized regulatory-document binary together
// with the trusted metadata required to serve it to a client.
//
// StorageKey is deliberately not exposed outside the service.
type OpenDocument struct {
	Document *models.DriverDocument
	Body     io.ReadCloser
}

// Open returns the binary for an active document only after Get has verified
// that the document belongs to the supplied operational driver.
func (s *Service) Open(
	ctx context.Context,
	driverID string,
	documentID string,
) (*OpenDocument, error) {
	document, err := s.Get(
		ctx,
		driverID,
		documentID,
	)
	if err != nil {
		return nil, err
	}

	if s.storage == nil {
		return nil, fmt.Errorf(
			"open driver document: storage dependency is required",
		)
	}

	body, err := s.storage.Open(
		ctx,
		document.StorageKey,
	)
	if errors.Is(err, documentstorage.ErrNotFound) {
		return nil, ErrDocumentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf(
			"open driver document binary: %w",
			err,
		)
	}

	return &OpenDocument{
		Document: document,
		Body:     body,
	}, nil
}

// OpenForUser resolves the authenticated users.id to its authoritative
// drivers.id before opening the requested regulatory-document binary.
func (s *Service) OpenForUser(
	ctx context.Context,
	userID string,
	documentID string,
) (*OpenDocument, error) {
	driverID, err := s.driverIDForUser(
		ctx,
		userID,
	)
	if err != nil {
		return nil, err
	}

	return s.Open(
		ctx,
		driverID,
		documentID,
	)
}
