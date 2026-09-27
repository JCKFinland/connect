package driver_document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

// Submit creates or replaces the driver's active regulatory document of the
// requested type.
//
// Replacement is atomic. The driver's row is locked for the duration of the
// transaction so concurrent submissions for the same driver are serialized.
func (s *Service) Submit(
	ctx context.Context,
	driverID string,
	req SubmitDocumentRequest,
) (*models.DriverDocument, error) {
	driverID = strings.TrimSpace(driverID)

	document, err := buildPendingDocument(
		driverID,
		req,
		time.Now(),
	)
	if err != nil {
		return nil, err
	}

	if s.db == nil {
		return nil, fmt.Errorf(
			"submit driver document: database dependency is required",
		)
	}

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			drivers := postgresrepo.NewDriverRepositoryWithDB(tx)
			documents := postgresrepo.NewDriverDocumentRepositoryWithDB(tx)

			// Lock the driver row first. This gives all document submissions
			// for the same driver a stable serialization point.
			_, err := drivers.GetByIDForUpdate(
				ctx,
				driverID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotFound
			}
			if err != nil {
				return fmt.Errorf(
					"lock driver for document submission: %w",
					err,
				)
			}

			existing, err := documents.GetByDriverAndType(
				ctx,
				driverID,
				document.DocumentType,
			)
			switch {
			case err == nil:
				if err := documents.SoftDelete(
					ctx,
					existing.ID,
				); err != nil {
					return fmt.Errorf(
						"replace driver document: soft delete existing document: %w",
						err,
					)
				}

			case errors.Is(err, repository.ErrNotFound):
				// No active document of this type exists. This is the
				// normal first-submission path.

			default:
				return fmt.Errorf(
					"replace driver document: get existing document: %w",
					err,
				)
			}

			if err := documents.Create(
				ctx,
				document,
			); err != nil {
				return fmt.Errorf(
					"create driver document: %w",
					err,
				)
			}

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return document, nil
}

func buildPendingDocument(
	driverID string,
	req SubmitDocumentRequest,
	now time.Time,
) (*models.DriverDocument, error) {
	driverID = strings.TrimSpace(driverID)

	documentType := strings.ToUpper(
		strings.TrimSpace(req.DocumentType),
	)
	fileName := strings.TrimSpace(req.FileName)
	storageKey := strings.TrimSpace(req.StorageKey)
	contentType := strings.TrimSpace(req.ContentType)

	if driverID == "" {
		return nil, fmt.Errorf(
			"%w: driver id is required",
			ErrInvalidDocument,
		)
	}

	switch documentType {
	case models.DriverDocumentTypeTaxiDriverLicense,
		models.DriverDocumentTypeDrivingLicense:
		// Supported regulatory document types.

	default:
		return nil, fmt.Errorf(
			"%w: unsupported document type",
			ErrInvalidDocument,
		)
	}

	if fileName == "" {
		return nil, fmt.Errorf(
			"%w: file name is required",
			ErrInvalidDocument,
		)
	}

	if utf8.RuneCountInString(fileName) > 255 {
		return nil, fmt.Errorf(
			"%w: file name must not exceed 255 characters",
			ErrInvalidDocument,
		)
	}

	if storageKey == "" {
		return nil, fmt.Errorf(
			"%w: storage key is required",
			ErrInvalidDocument,
		)
	}

	if contentType == "" {
		return nil, fmt.Errorf(
			"%w: content type is required",
			ErrInvalidDocument,
		)
	}

	if len(contentType) > 100 {
		return nil, fmt.Errorf(
			"%w: content type must not exceed 100 characters",
			ErrInvalidDocument,
		)
	}

	if req.FileSizeBytes <= 0 {
		return nil, fmt.Errorf(
			"%w: file size must be greater than zero",
			ErrInvalidDocument,
		)
	}

	if req.ExpiresAt == nil {
		return nil, fmt.Errorf(
			"%w: expiry date is required",
			ErrInvalidDocument,
		)
	}

	expiryDate := dateOnly(*req.ExpiresAt)
	today := dateOnly(now)

	if !expiryDate.After(today) {
		return nil, fmt.Errorf(
			"%w: expiry date must be in the future",
			ErrInvalidDocument,
		)
	}

	return &models.DriverDocument{
		DriverID:      driverID,
		DocumentType:  documentType,
		FileName:      fileName,
		StorageKey:    storageKey,
		ContentType:   contentType,
		FileSizeBytes: req.FileSizeBytes,
		ExpiresAt:     &expiryDate,

		Status: models.DriverDocumentStatusPending,

		VerifiedAt:       nil,
		VerifiedByUserID: nil,
		RejectionReason:  nil,
	}, nil
}

func dateOnly(
	value time.Time,
) time.Time {
	return time.Date(
		value.Year(),
		value.Month(),
		value.Day(),
		0,
		0,
		0,
		0,
		value.Location(),
	)
}
