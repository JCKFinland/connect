package presence

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var ErrDriverAvailabilityLocked = errors.New(
	"driver availability cannot be changed while committed to an active trip",
)

var ErrInvalidAvailabilityStatus = errors.New(
	"invalid driver availability status",
)

func (s *Service) UpdateAvailability(
	ctx context.Context,
	req UpdateAvailabilityRequest,
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

	// ---------------------------------------------------------
	// 1. Validate driver-controlled availability state before
	//    starting any persistence work.
	// ---------------------------------------------------------

	switch req.Status {

	case StatusAvailable,
		StatusBreak,
		StatusOffDuty:

		// Driver-controlled availability states.

	default:
		return ErrInvalidAvailabilityStatus
	}

	// ---------------------------------------------------------
	// 2. Lock presence and mutate availability atomically.
	// ---------------------------------------------------------

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			presenceRepo :=
				postgresrepo.NewDriverPresenceRepositoryWithDB(
					tx,
				)

			// ---------------------------------------------------------
			// Lock the driver's lifecycle row first.
			//
			// This lets us distinguish:
			//
			//   missing presence
			//       -> ErrDriverNotFound
			//
			//   BUSY / active trip
			//       -> ErrDriverAvailabilityLocked
			//
			// The same presence row is also used as the serialization
			// point by dispatch and assignment lifecycle operations.
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
					"lock driver presence before availability update: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// Apply the requested manual state only if the driver
			// remains operationally idle.
			// ---------------------------------------------------------

			updated, err :=
				presenceRepo.UpdateAvailabilityIfIdle(
					ctx,
					req.UserID,
					req.Status,
					true,
				)

			if err != nil {
				return fmt.Errorf(
					"update driver availability: %w",
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
