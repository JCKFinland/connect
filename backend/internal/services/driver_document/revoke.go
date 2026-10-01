package driver_document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

// Revoke transitions an active VERIFIED driver document to REVOKED.
//
// reviewerUserID is the authenticated administrator's users.id. The original
// verification audit is preserved; revocation records a separate audit event.
func (s *Service) Revoke(
	ctx context.Context,
	documentID string,
	reviewerUserID string,
	reason string,
) (*models.DriverDocument, error) {
	documentID = strings.TrimSpace(documentID)
	reviewerUserID = strings.TrimSpace(reviewerUserID)
	reason = strings.TrimSpace(reason)

	if documentID == "" ||
		reviewerUserID == "" ||
		reason == "" {

		return nil, ErrInvalidDocument
	}

	if s.db == nil {
		return nil, fmt.Errorf(
			"driver document database is not configured",
		)
	}

	var revoked *models.DriverDocument

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			drivers :=
				postgresrepo.NewDriverRepositoryWithDB(tx)
			documents :=
				postgresrepo.NewDriverDocumentRepositoryWithDB(tx)

			// This first read discovers only the owning permanent driver.
			// Authoritative lifecycle state is re-read after the aggregate lock.
			documentIdentity, err := documents.GetByID(
				ctx,
				documentID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDocumentNotFound
			}
			if err != nil {
				return fmt.Errorf(
					"resolve driver document for revocation: %w",
					err,
				)
			}

			if documentIdentity == nil ||
				documentIdentity.DriverID == "" {

				return ErrDocumentNotFound
			}

			// Every regulatory eligibility mutation serializes on the permanent
			// driver row. Keep the canonical lock order: driver -> document.
			_, err = drivers.GetByIDForUpdate(
				ctx,
				documentIdentity.DriverID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotFound
			}
			if err != nil {
				return fmt.Errorf(
					"lock driver for document revocation: %w",
					err,
				)
			}

			document, err := documents.GetByIDForUpdate(
				ctx,
				documentID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDocumentNotFound
			}
			if err != nil {
				return fmt.Errorf(
					"lock driver document for revocation: %w",
					err,
				)
			}

			if document == nil ||
				document.DriverID != documentIdentity.DriverID {

				return ErrDocumentNotFound
			}

			if document.Status !=
				models.DriverDocumentStatusVerified {

				return ErrDocumentNotRevocable
			}

			now := time.Now().UTC()

			updatedAt, err := documents.UpdateRevocationState(
				ctx,
				document.ID,
				now,
				reviewerUserID,
				reason,
			)
			if err != nil {
				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
					return ErrDocumentNotFound
				}

				return fmt.Errorf(
					"update driver document revocation state: %w",
					err,
				)
			}

			document.Status =
				models.DriverDocumentStatusRevoked
			document.RevokedAt = &now
			document.RevokedByUserID = &reviewerUserID
			document.RevocationReason = &reason
			document.UpdatedAt = updatedAt

			revoked = document

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return revoked, nil
}
