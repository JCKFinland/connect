package driver_document

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Get returns an active driver document only when it belongs to the supplied
// operational driver.
//
// A document belonging to another driver is intentionally reported as not
// found so callers cannot use document IDs to discover another driver's
// regulatory-document metadata.
func (s *Service) Get(
	ctx context.Context,
	driverID string,
	documentID string,
) (*models.DriverDocument, error) {
	driverID = strings.TrimSpace(driverID)
	documentID = strings.TrimSpace(documentID)

	if driverID == "" || documentID == "" {
		return nil, ErrInvalidDocument
	}

	if s.documents == nil {
		return nil, fmt.Errorf(
			"driver document repository is not configured",
		)
	}

	document, err := s.documents.GetByID(
		ctx,
		documentID,
	)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrDocumentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf(
			"get driver document: %w",
			err,
		)
	}

	if document == nil ||
		document.DriverID != driverID {
		return nil, ErrDocumentNotFound
	}

	return document, nil
}
