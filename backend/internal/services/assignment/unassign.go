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
	req UnassignDriverRequest,
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

	if req.DriverID == "" {
		return errors.New(
			"driver ID is required",
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
			// 1. Lock the driver's presence row.
			//
			// AcceptOffer() also locks this row before committing a
			// driver to a trip. Sharing this lock serializes acceptance
			// and assignment removal.
			// ---------------------------------------------------------

			if _, err :=
				presenceRepo.GetByDriverIDForUpdate(
					ctx,
					req.DriverID,
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

			_, err :=
				assignments.GetActiveByDriver(
					ctx,
					req.DriverID,
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
					req.DriverID,
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
			// 4. Close the assignment.
			//
			// Both this update and the presence detachment are inside
			// the same PostgreSQL transaction.
			// ---------------------------------------------------------

			if err := assignments.CloseAssignment(
				ctx,
				req.DriverID,
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
