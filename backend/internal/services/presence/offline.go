package presence

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

func (s *Service) GoOffline(
	ctx context.Context,
	req GoOfflineRequest,
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

			// ---------------------------------------------------------
			// 1. Lock the driver's presence lifecycle row.
			//
			// This distinguishes a genuinely missing presence record
			// from a BUSY/active-trip lifecycle conflict and also
			// serializes GoOffline with assignment/dispatch operations.
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
					"lock driver presence before going offline: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 2. Transition to OFFLINE only when the driver remains
			//    operationally idle.
			//
			// BUSY and active-trip state must never be overwritten.
			// ---------------------------------------------------------

			updated, err :=
				presenceRepo.UpdateAvailabilityIfIdle(
					ctx,
					req.UserID,
					StatusOffline,
					false,
				)

			if err != nil {
				return fmt.Errorf(
					"mark driver offline: %w",
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
