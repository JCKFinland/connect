package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// GetByIDForUpdate returns an active driver document and locks its row for the
// duration of the current PostgreSQL transaction.
//
// The lock serializes competing regulatory-review decisions so that two
// reviewers cannot independently resolve the same PENDING document.
func (r *DriverDocumentRepository) GetByIDForUpdate(
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
		FOR UPDATE
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
			"get driver document by id for update: %w",
			err,
		)
	}

	return &document, nil
}

// UpdateReviewState persists the regulatory-review state of an active driver
// document.
//
// Lifecycle validation belongs to the service. This repository method only
// persists a review decision that has already been validated while the
// document row is locked by the surrounding transaction.
func (r *DriverDocumentRepository) UpdateReviewState(
	ctx context.Context,
	id string,
	status string,
	verifiedAt *time.Time,
	verifiedByUserID string,
	rejectionReason *string,
) (time.Time, error) {
	const query = `
		UPDATE driver_documents
		SET
			status = $2,
			verified_at = $3,
			verified_by_user_id = $4,
			rejection_reason = $5,
			updated_at = NOW()
		WHERE id = $1
		  AND deleted_at IS NULL
		RETURNING updated_at
	`

	var updatedAt time.Time

	err := r.db.QueryRow(
		ctx,
		query,
		id,
		status,
		verifiedAt,
		verifiedByUserID,
		rejectionReason,
	).Scan(
		&updatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, repository.ErrNotFound
	}

	if err != nil {
		return time.Time{}, fmt.Errorf(
			"update driver document review state: %w",
			err,
		)
	}

	return updatedAt, nil
}

// UpdateRevocationState persists the revocation of a previously verified
// regulatory document.
//
// Lifecycle validation belongs to the service. The surrounding transaction
// must hold the driver's aggregate lock and the document row lock before this
// mutation is executed. Verification audit fields are deliberately preserved.
func (r *DriverDocumentRepository) UpdateRevocationState(
	ctx context.Context,
	id string,
	revokedAt time.Time,
	revokedByUserID string,
	revocationReason string,
) (time.Time, error) {
	const query = `
		UPDATE driver_documents
		SET
			status = 'REVOKED',
			revoked_at = $2,
			revoked_by_user_id = $3,
			revocation_reason = $4,
			updated_at = NOW()
		WHERE id = $1
		  AND deleted_at IS NULL
		RETURNING updated_at
	`

	var updatedAt time.Time

	err := r.db.QueryRow(
		ctx,
		query,
		id,
		revokedAt,
		revokedByUserID,
		revocationReason,
	).Scan(
		&updatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, repository.ErrNotFound
	}

	if err != nil {
		return time.Time{}, fmt.Errorf(
			"update driver document revocation state: %w",
			err,
		)
	}

	return updatedAt, nil
}
