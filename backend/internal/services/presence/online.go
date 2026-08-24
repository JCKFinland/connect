package presence

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var ErrDriverAssignmentRequired = errors.New(
	"driver assignment required before going online",
)

func (s *Service) GoOnline(
	ctx context.Context,
	req GoOnlineRequest,
) error {

	if s == nil {
		return errors.New(
			"presence service is required",
		)
	}

	if s.db == nil {
		return errors.New(
			"presence database is not configured",
		)
	}

	if req.UserID == "" {
		return errors.New(
			"user ID is required",
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			presenceRepo :=
				postgresrepo.NewDriverPresenceRepositoryWithDB(
					tx,
				)

			assignmentRepo :=
				postgresrepo.NewDriverAssignmentRepositoryWithDB(
					tx,
				)

			// ---------------------------------------------------------
			// 1. Lock the driver's lifecycle state.
			//
			// AcceptOffer(), Assign(), and Unassign() use this same
			// presence row as their serialization point.
			// ---------------------------------------------------------

			_, err :=
				presenceRepo.GetByDriverIDForUpdate(
					ctx,
					req.UserID,
				)

			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return ErrDriverNotFound
			}

			if err != nil {
				return fmt.Errorf(
					"lock driver presence before going online: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 2. Load the authoritative active vehicle assignment
			//    while the driver lifecycle row remains locked.
			// ---------------------------------------------------------

			assignment, err :=
				assignmentRepo.GetActiveByDriver(
					ctx,
					req.UserID,
				)

			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return ErrDriverAssignmentRequired
			}

			if err != nil {
				return fmt.Errorf(
					"get active driver assignment before going online: %w",
					err,
				)
			}

			if assignment == nil ||
				assignment.ID == "" ||
				assignment.VehicleID == "" {

				return ErrDriverAssignmentRequired
			}

			// ---------------------------------------------------------
			// 3. Reconcile assignment data into driver_presence.
			//
			// This is guarded against BUSY/active-trip state.
			// The driver's existing online/offline availability is
			// deliberately preserved by this operation.
			// ---------------------------------------------------------

			attached, err :=
				presenceRepo.AttachAssignmentIfIdle(
					ctx,
					req.UserID,
					assignment.CompanyID,
					assignment.BranchID,
					assignment.VehicleID,
					assignment.ID,
				)

			if err != nil {
				return fmt.Errorf(
					"attach active assignment before going online: %w",
					err,
				)
			}

			if !attached {
				return ErrDriverAvailabilityLocked
			}

			// ---------------------------------------------------------
			// 4. Transition to ONLINE + AVAILABLE.
			//
			// Guard the final state transition as well. Both this and
			// assignment reconciliation belong to the same transaction.
			// ---------------------------------------------------------

			updated, err :=
				presenceRepo.UpdateAvailabilityIfIdle(
					ctx,
					req.UserID,
					StatusAvailable,
					true,
				)

			if err != nil {
				return fmt.Errorf(
					"mark driver available while going online: %w",
					err,
				)
			}

			if !updated {
				return ErrDriverAvailabilityLocked
			}

			return nil
		},
	)
}
