package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// DriverDocumentRepository persists driver regulatory-document metadata.
type DriverDocumentRepository struct {
	db DBTX
}

// Compile-time check that DriverDocumentRepository satisfies the
// repository.DriverDocumentRepository contract.
var _ repository.DriverDocumentRepository = (*DriverDocumentRepository)(nil)

// NewDriverDocumentRepository creates a driver-document repository backed
// by the application's PostgreSQL connection pool.
func NewDriverDocumentRepository(
	db *pgxpool.Pool,
) *DriverDocumentRepository {
	return &DriverDocumentRepository{
		db: db,
	}
}

// NewDriverDocumentRepositoryWithDB creates a driver-document repository
// backed by any DBTX implementation, including pgx.Tx.
//
// This constructor is used when driver-document operations must participate
// in an existing PostgreSQL transaction.
func NewDriverDocumentRepositoryWithDB(
	db DBTX,
) *DriverDocumentRepository {
	return &DriverDocumentRepository{
		db: db,
	}
}

// Create persists driver regulatory-document metadata.
//
// The actual document binary is not stored in PostgreSQL. StorageKey points
// to the document in the configured object/file storage system.
func (r *DriverDocumentRepository) Create(
	ctx context.Context,
	document *models.DriverDocument,
) error {
	const query = `
		INSERT INTO driver_documents (
			driver_id,
			document_type,
			file_name,
			storage_key,
			content_type,
			file_size_bytes,
			expires_at,
			status,
			verified_at,
			verified_by_user_id,
			rejection_reason
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11
		)
		RETURNING
			id,
			created_at,
			updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		document.DriverID,
		document.DocumentType,
		document.FileName,
		document.StorageKey,
		document.ContentType,
		document.FileSizeBytes,
		document.ExpiresAt,
		document.Status,
		document.VerifiedAt,
		document.VerifiedByUserID,
		document.RejectionReason,
	).Scan(
		&document.ID,
		&document.CreatedAt,
		&document.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf(
			"create driver document: %w",
			err,
		)
	}

	return nil
}

// GetByID returns one active driver document by its document ID.
//
// Soft-deleted documents are deliberately excluded.
//
// PostgreSQL's pgx.ErrNoRows is translated into repository.ErrNotFound so
// callers do not need to depend on PostgreSQL-specific error semantics.
func (r *DriverDocumentRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.DriverDocument, error) {
	const query = `
		SELECT
			id,
			driver_id,
			document_type,
			file_name,
			storage_key,
			content_type,
			file_size_bytes,
			expires_at,
			status,
			verified_at,
			verified_by_user_id,
			rejection_reason,
			revoked_at,
			revoked_by_user_id,
			revocation_reason,
			created_at,
			updated_at,
			deleted_at
		FROM driver_documents
		WHERE id = $1
		  AND deleted_at IS NULL
	`

	var document models.DriverDocument

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&document.ID,
		&document.DriverID,
		&document.DocumentType,
		&document.FileName,
		&document.StorageKey,
		&document.ContentType,
		&document.FileSizeBytes,
		&document.ExpiresAt,
		&document.Status,
		&document.VerifiedAt,
		&document.VerifiedByUserID,
		&document.RejectionReason,
		&document.RevokedAt,
		&document.RevokedByUserID,
		&document.RevocationReason,
		&document.CreatedAt,
		&document.UpdatedAt,
		&document.DeletedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get driver document by id: %w",
			err,
		)
	}

	return &document, nil
}

// GetByDriverAndType returns the active document of a specific regulatory
// document type belonging to a driver.
//
// The database partial unique index guarantees that at most one active
// document exists for a given (driver_id, document_type) pair.
func (r *DriverDocumentRepository) GetByDriverAndType(
	ctx context.Context,
	driverID string,
	documentType string,
) (*models.DriverDocument, error) {
	const query = `
		SELECT
			id,
			driver_id,
			document_type,
			file_name,
			storage_key,
			content_type,
			file_size_bytes,
			expires_at,
			status,
			verified_at,
			verified_by_user_id,
			rejection_reason,
			revoked_at,
			revoked_by_user_id,
			revocation_reason,
			created_at,
			updated_at,
			deleted_at
		FROM driver_documents
		WHERE driver_id = $1
		  AND document_type = $2
		  AND deleted_at IS NULL
	`

	var document models.DriverDocument

	err := r.db.QueryRow(
		ctx,
		query,
		driverID,
		documentType,
	).Scan(
		&document.ID,
		&document.DriverID,
		&document.DocumentType,
		&document.FileName,
		&document.StorageKey,
		&document.ContentType,
		&document.FileSizeBytes,
		&document.ExpiresAt,
		&document.Status,
		&document.VerifiedAt,
		&document.VerifiedByUserID,
		&document.RejectionReason,
		&document.RevokedAt,
		&document.RevokedByUserID,
		&document.RevocationReason,
		&document.CreatedAt,
		&document.UpdatedAt,
		&document.DeletedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get driver document by driver and type: %w",
			err,
		)
	}

	return &document, nil
}

// ListByDriver returns all active regulatory documents belonging to a driver.
func (r *DriverDocumentRepository) ListByDriver(
	ctx context.Context,
	driverID string,
) ([]models.DriverDocument, error) {
	const query = `
		SELECT
			id,
			driver_id,
			document_type,
			file_name,
			storage_key,
			content_type,
			file_size_bytes,
			expires_at,
			status,
			verified_at,
			verified_by_user_id,
			rejection_reason,
			revoked_at,
			revoked_by_user_id,
			revocation_reason,
			created_at,
			updated_at,
			deleted_at
		FROM driver_documents
		WHERE driver_id = $1
		  AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		driverID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list driver documents: %w",
			err,
		)
	}
	defer rows.Close()

	documents := make(
		[]models.DriverDocument,
		0,
	)

	for rows.Next() {
		var document models.DriverDocument

		if err := rows.Scan(
			&document.ID,
			&document.DriverID,
			&document.DocumentType,
			&document.FileName,
			&document.StorageKey,
			&document.ContentType,
			&document.FileSizeBytes,
			&document.ExpiresAt,
			&document.Status,
			&document.VerifiedAt,
			&document.VerifiedByUserID,
			&document.RejectionReason,
			&document.RevokedAt,
			&document.RevokedByUserID,
			&document.RevocationReason,
			&document.CreatedAt,
			&document.UpdatedAt,
			&document.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf(
				"scan driver document: %w",
				err,
			)
		}

		documents = append(
			documents,
			document,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate driver documents: %w",
			err,
		)
	}

	return documents, nil
}

// SoftDelete removes a driver document from the active document set without
// physically deleting its audit record.
//
// Returning repository.ErrNotFound when no active row was updated keeps the
// repository contract independent from pgx.
func (r *DriverDocumentRepository) SoftDelete(
	ctx context.Context,
	id string,
) error {
	const query = `
		UPDATE driver_documents
		SET
			deleted_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		id,
	)
	if err != nil {
		return fmt.Errorf(
			"soft delete driver document: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}
