package driver_document

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	documentstorage "github.com/JCKFinland/connect/backend/internal/storage/document"
)

// UploadDocumentRequest contains trusted upload metadata plus the binary stream
// for a driver regulatory document.
//
// StorageKey is deliberately absent. The service generates it server-side.
type UploadDocumentRequest struct {
	DocumentType string
	FileName     string
	ExpiresAt    *time.Time
	Body         io.Reader
}

// UploadForUser stores a regulatory-document binary and then persists its
// metadata for the driver associated with the authenticated platform user.
//
// If metadata persistence fails after storage succeeds, the newly written
// object is deleted as a compensating action.
func (s *Service) UploadForUser(
	ctx context.Context,
	userID string,
	req UploadDocumentRequest,
) (*models.DriverDocument, error) {
	driverID, err := s.driverIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	if s.storage == nil {
		return nil, fmt.Errorf(
			"upload driver document: storage dependency is required",
		)
	}

	if err := validateDocumentFileName(req.FileName); err != nil {
		return nil, err
	}

	validatedBinary, err := validateDocumentBinary(
		req.Body,
		s.uploadMaxBytes,
	)
	if err != nil {
		return nil, err
	}

	key, err := documentstorage.NewDriverDocumentKey(
		driverID,
		req.DocumentType,
		validatedBinary.Extension,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: generate storage key: %v",
			ErrInvalidDocument,
			err,
		)
	}

	if err := s.storage.Put(
		ctx,
		documentstorage.PutRequest{
			Key:  key,
			Body: validatedBinary.Body,
		},
	); err != nil {
		return nil, fmt.Errorf(
			"store driver document binary: %w",
			err,
		)
	}

	document, submitErr := s.Submit(
		ctx,
		driverID,
		SubmitDocumentRequest{
			DocumentType:  req.DocumentType,
			FileName:      req.FileName,
			StorageKey:    key,
			ContentType:   validatedBinary.ContentType,
			FileSizeBytes: validatedBinary.FileSizeBytes,
			ExpiresAt:     req.ExpiresAt,
		},
	)
	if submitErr == nil {
		return document, nil
	}

	// The database did not accept the metadata, so the newly stored object
	// must not remain orphaned. Use a cancellation-independent context:
	// request cancellation may itself be the reason Submit failed.
	cleanupCtx := context.WithoutCancel(ctx)

	deleteErr := s.storage.Delete(
		cleanupCtx,
		key,
	)
	if deleteErr != nil &&
		!errors.Is(
			deleteErr,
			documentstorage.ErrNotFound,
		) {
		return nil, errors.Join(
			submitErr,
			fmt.Errorf(
				"cleanup stored driver document after persistence failure: %w",
				deleteErr,
			),
		)
	}

	return nil, submitErr
}
