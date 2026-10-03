package trip

import (
	"context"
	"errors"
	"fmt"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var ErrTripDeleteRequiresTerminalStatus = errors.New(
	"trip must be completed or cancelled before deletion",
)

// Delete soft-deletes a trip.
func (s *tripService) Delete(
	ctx context.Context,
	id string,
) error {

	if id == "" {
		return fmt.Errorf("trip ID is required")
	}

	return s.repo.Delete(
		ctx,
		id,
	)
}

// DeleteAuthorized soft-deletes a terminal trip only when the authenticated
// user has operational trip-management privileges.
func (s *tripService) DeleteAuthorized(
	ctx context.Context,
	id string,
	userID string,
) error {

	if id == "" {
		return fmt.Errorf("trip ID is required")
	}

	if err := s.authorizeOperationalMutation(
		ctx,
		userID,
	); err != nil {
		return err
	}

	if s.db == nil {
		return fmt.Errorf("trip database is not configured")
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			trips := postgresrepo.NewTripRepositoryWithDB(tx)

			currentTrip, err := trips.GetByIDForUpdate(
				ctx,
				id,
			)
			if err != nil {
				return fmt.Errorf(
					"get trip for deletion: %w",
					err,
				)
			}

			if currentTrip.Status != StatusCompleted &&
				currentTrip.Status != StatusCancelled {
				return ErrTripDeleteRequiresTerminalStatus
			}

			if err := trips.Delete(
				ctx,
				currentTrip.ID,
			); err != nil {
				return fmt.Errorf(
					"delete terminal trip: %w",
					err,
				)
			}

			return nil
		},
	)
}
