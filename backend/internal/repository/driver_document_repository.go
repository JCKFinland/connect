package repository

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// DriverDocumentRepository defines persistence operations for driver
// regulatory document metadata.
type DriverDocumentRepository interface {
	// Create stores a new driver document record.
	Create(
		ctx context.Context,
		document *models.DriverDocument,
	) error

	// GetByID returns an active driver document by ID.
	GetByID(
		ctx context.Context,
		id string,
	) (*models.DriverDocument, error)

	// GetByIDForUpdate returns an active driver document by ID and locks
	// the row for the duration of the current PostgreSQL transaction.
	GetByIDForUpdate(
		ctx context.Context,
		id string,
	) (*models.DriverDocument, error)

	// GetByDriverAndType returns the driver's current active document
	// for the requested document type.
	GetByDriverAndType(
		ctx context.Context,
		driverID string,
		documentType string,
	) (*models.DriverDocument, error)

	// ListByDriver returns the driver's active document records.
	ListByDriver(
		ctx context.Context,
		driverID string,
	) ([]models.DriverDocument, error)

	// UpdateReviewState persists a terminal regulatory-review decision.
	UpdateReviewState(
		ctx context.Context,
		id string,
		status string,
		verifiedAt *time.Time,
		verifiedByUserID string,
		rejectionReason *string,
	) (time.Time, error)

	// SoftDelete marks a document as deleted.
	SoftDelete(
		ctx context.Context,
		id string,
	) error
}
