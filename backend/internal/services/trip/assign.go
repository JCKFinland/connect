package trip

import (
	"context"
	"errors"
	"fmt"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// ErrTripAssignmentValidation identifies invalid driver-assignment
// input or a disallowed trip lifecycle state.
var ErrTripAssignmentValidation = errors.New(
	"invalid trip assignment operation",
)

// AssignDriver assigns a driver and vehicle to a trip.
func (s *tripService) AssignDriver(
	ctx context.Context,
	id string,
	driverID string,
	vehicleID string,
) error {

	if id == "" {
		return fmt.Errorf("%w: trip ID is required", ErrTripAssignmentValidation)
	}

	if driverID == "" {
		return fmt.Errorf("%w: driver ID is required", ErrTripAssignmentValidation)
	}

	if vehicleID == "" {
		return fmt.Errorf("%w: vehicle ID is required", ErrTripAssignmentValidation)
	}

	currentTrip, err := s.repo.GetByID(
		ctx,
		id,
	)
	if err != nil {
		return fmt.Errorf(
			"get trip for assignment: %w",
			err,
		)
	}

	if currentTrip.Status != StatusAssigned {
		return fmt.Errorf(
			"%w: cannot assign driver to trip in status %s",
			ErrTripAssignmentValidation,
			currentTrip.Status,
		)
	}

	return s.repo.AssignDriver(
		ctx,
		id,
		driverID,
		vehicleID,
	)
}

// AssignDriverAuthorized assigns a driver and vehicle only when the
// authenticated user has operational trip-management privileges.
func (s *tripService) AssignDriverAuthorized(
	ctx context.Context,
	id string,
	driverID string,
	vehicleID string,
	userID string,
) error {
	if id == "" {
		return fmt.Errorf("%w: trip ID is required", ErrTripAssignmentValidation)
	}
	if driverID == "" {
		return fmt.Errorf("%w: driver ID is required", ErrTripAssignmentValidation)
	}
	if vehicleID == "" {
		return fmt.Errorf("%w: vehicle ID is required", ErrTripAssignmentValidation)
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

	roles, err := s.userRoles.GetUserRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf(
			"get user roles for driver assignment: %w",
			err,
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			trips := postgresrepo.NewTripRepositoryWithDB(tx)

			currentTrip, err := trips.GetByIDForUpdate(ctx, id)
			if err != nil {
				return fmt.Errorf(
					"get trip for assignment: %w",
					err,
				)
			}

			allowed, err := s.hasOperationalCompanyAuthorityForUpdate(
				ctx,
				tx, userID,
				currentTrip.CompanyID,
				roles,
			)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrTripMutationAccessDenied
			}

			if currentTrip.Status != StatusAssigned {
				return fmt.Errorf(
					"%w: cannot assign driver to trip in status %s",
					ErrTripAssignmentValidation,
					currentTrip.Status,
				)
			}

			if err := trips.AssignDriver(
				ctx,
				id,
				driverID,
				vehicleID,
			); err != nil {
				return fmt.Errorf(
					"assign authorized trip driver: %w",
					err,
				)
			}

			return nil
		},
	)
}
