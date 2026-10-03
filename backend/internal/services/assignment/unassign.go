package assignment

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/jackc/pgx/v5"
)

func (s *Service) Unassign(
	ctx context.Context,
	userID string,
) error {

	if s == nil {
		return errors.New(
			"assignment service is required",
		)
	}

	if s.db == nil {
		return errors.New(
			"assignment database is not configured",
		)
	}

	if userID == "" {
		return errors.New(
			"authenticated user ID is required",
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			assignments :=
				postgresrepo.NewDriverAssignmentRepositoryWithDB(
					tx,
				)

			presenceRepo :=
				postgresrepo.NewDriverPresenceRepositoryWithDB(
					tx,
				)

			// ---------------------------------------------------------
			// 1. Lock the authenticated driver's presence row.
			//
			// AcceptOffer() also locks this row before committing a
			// driver to a trip. Sharing this lock serializes acceptance
			// and assignment removal.
			// ---------------------------------------------------------

			if _, err :=
				presenceRepo.GetByDriverIDForUpdate(
					ctx,
					userID,
				); err != nil {

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
					return ErrAssignmentNotFound
				}

				return fmt.Errorf(
					"lock driver presence for unassignment: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 2. Confirm an active assignment still exists after the
			//    lifecycle lock has been acquired.
			// ---------------------------------------------------------

			activeAssignment, err :=
				assignments.GetActiveByDriver(
					ctx,
					userID,
				)

			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return ErrAssignmentNotFound
			}

			if err != nil {
				return fmt.Errorf(
					"get active driver assignment: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 3. Attempt guarded presence detachment BEFORE closing
			//    the assignment.
			//
			// If BUSY/active-trip protection rejects this operation,
			// nothing has been changed yet.
			// ---------------------------------------------------------

			detached, err :=
				presenceRepo.DetachAssignmentIfIdle(
					ctx,
					userID,
				)

			if err != nil {
				return fmt.Errorf(
					"detach driver presence assignment: %w",
					err,
				)
			}

			if !detached {
				return presence.ErrDriverAvailabilityLocked
			}

			// ---------------------------------------------------------
			// 4. Close the exact assignment selected under the same
			//    driver lifecycle lock.
			// ---------------------------------------------------------

			if err := assignments.CloseAssignment(
				ctx,
				activeAssignment.ID,
			); err != nil {

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
					return ErrAssignmentNotFound
				}

				return fmt.Errorf(
					"close driver assignment: %w",
					err,
				)
			}

			return nil
		},
	)
}
