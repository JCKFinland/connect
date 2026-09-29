package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// MakeDriverRegulatorilyCompliant adds the verified regulatory documents
// required for an existing DriverFixture to pass dispatch compliance checks.
//
// The returned cleanup function must be called before the DriverFixture
// cleanup function because driver_documents references drivers.
func MakeDriverRegulatorilyCompliant(
	ctx context.Context,
	db *pgxpool.Pool,
	fixture *DriverFixture,
) (func(context.Context) error, error) {
	if db == nil {
		return nil, fmt.Errorf("database pool is required")
	}
	if fixture == nil {
		return nil, fmt.Errorf("driver fixture is required")
	}
	if fixture.DriverID == "" {
		return nil, fmt.Errorf("driver fixture driver ID is required")
	}
	if fixture.UserID == "" {
		return nil, fmt.Errorf("driver fixture user ID is required")
	}

	now := time.Now().UTC()
	expiresAt := now.AddDate(1, 0, 0)

	documentTypes := []string{
		models.DriverDocumentTypeDrivingLicense,
		models.DriverDocumentTypeTaxiDriverLicense,
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"begin driver compliance fixture creation: %w",
			err,
		)
	}

	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	documentIDs := make([]string, 0, len(documentTypes))

	for _, documentType := range documentTypes {
		documentID := uuid.NewString()

		_, err := tx.Exec(
			ctx,
			`
				INSERT INTO driver_documents (
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
					verified_by_user_id
				)
				VALUES (
					$1,
					$2,
					$3,
					$4,
					$5,
					$6,
					$7,
					$8,
					$9,
					$10,
					$11
				)
			`,
			documentID,
			fixture.DriverID,
			documentType,
			"test-"+documentType+".pdf",
			"test/driver-documents/"+uuid.NewString()+".pdf",
			"application/pdf",
			int64(1),
			expiresAt,
			models.DriverDocumentStatusVerified,
			now,
			fixture.UserID,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"create verified %s document: %w",
				documentType,
				err,
			)
		}

		documentIDs = append(documentIDs, documentID)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf(
			"commit driver compliance fixture creation: %w",
			err,
		)
	}

	cleanup := func(cleanupCtx context.Context) error {
		tx, err := db.Begin(cleanupCtx)
		if err != nil {
			return fmt.Errorf(
				"begin driver compliance fixture cleanup: %w",
				err,
			)
		}

		defer func() {
			_ = tx.Rollback(context.Background())
		}()

		for _, documentID := range documentIDs {
			if _, err := tx.Exec(
				cleanupCtx,
				`DELETE FROM driver_documents WHERE id = $1`,
				documentID,
			); err != nil {
				return fmt.Errorf(
					"delete test driver document %s: %w",
					documentID,
					err,
				)
			}
		}

		if err := tx.Commit(cleanupCtx); err != nil {
			return fmt.Errorf(
				"commit driver compliance fixture cleanup: %w",
				err,
			)
		}

		return nil
	}

	return cleanup, nil
}
