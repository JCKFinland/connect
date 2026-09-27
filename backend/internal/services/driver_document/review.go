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

// Verify transitions an active PENDING driver document to VERIFIED.
//
// reviewerUserID is the authenticated reviewer's users.id.
func (s *Service) Verify(
	ctx context.Context,
	documentID string,
	reviewerUserID string,
) (*models.DriverDocument, error) {
	documentID = strings.TrimSpace(documentID)
	reviewerUserID = strings.TrimSpace(reviewerUserID)

	if documentID == "" || reviewerUserID == "" {
		return nil, ErrInvalidDocument
	}

	return s.review(
		ctx,
		documentID,
		reviewerUserID,
		models.DriverDocumentStatusVerified,
		nil,
	)
}

// Reject transitions an active PENDING driver document to REJECTED.
//
// reviewerUserID is the authenticated reviewer's users.id.
// A rejection reason is mandatory so that the regulatory decision is
// explainable to the driver and auditable later.
func (s *Service) Reject(
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

	return s.review(
		ctx,
		documentID,
		reviewerUserID,
		models.DriverDocumentStatusRejected,
		&reason,
	)
}

func (s *Service) review(
	ctx context.Context,
	documentID string,
	reviewerUserID string,
	status string,
	rejectionReason *string,
) (*models.DriverDocument, error) {
	if s.db == nil {
		return nil, fmt.Errorf(
			"driver document database is not configured",
		)
	}

	var reviewed *models.DriverDocument

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			documents :=
				postgresrepo.NewDriverDocumentRepositoryWithDB(tx)

			document, err := documents.GetByIDForUpdate(
				ctx,
				documentID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDocumentNotFound
			}
			if err != nil {
				return fmt.Errorf(
					"lock driver document for review: %w",
					err,
				)
			}

			if document == nil {
				return ErrDocumentNotFound
			}

			if document.Status !=
				models.DriverDocumentStatusPending {

				return ErrDocumentAlreadyReviewed
			}

			now := time.Now().UTC()

			var verifiedAt *time.Time
			if status ==
				models.DriverDocumentStatusVerified {

				verifiedAt = &now
			}

			updatedAt, err := documents.UpdateReviewState(
				ctx,
				document.ID,
				status,
				verifiedAt,
				reviewerUserID,
				rejectionReason,
			)
			if err != nil {
				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
					return ErrDocumentNotFound
				}

				return fmt.Errorf(
					"update driver document review state: %w",
					err,
				)
			}

			document.Status = status
			document.VerifiedAt = verifiedAt
			document.VerifiedByUserID = &reviewerUserID
			document.RejectionReason = rejectionReason
			document.UpdatedAt = updatedAt

			reviewed = document

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return reviewed, nil
}
