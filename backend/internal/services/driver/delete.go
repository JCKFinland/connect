package driver

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Delete archives a driver only when the driver has no live operational state.
//
// Lock order:
//
//	driver row -> presence row -> active assignment/trip checks -> archive
//
// The driver row is the lifecycle serialization point shared with assignment
// startup. The presence row, when it exists, serializes against the remaining
// operational presence/assignment transitions.
func (s *Service) Delete(
	ctx context.Context,
	userID string,
	id string,
) error {
	if userID == "" || id == "" {
		return ErrDriverNotFound
	}

	if s == nil || s.db == nil {
		return fmt.Errorf("driver database is not configured")
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			drivers := postgresrepo.NewDriverRepositoryWithDB(tx)
			presence := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)
			assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)
			trips := postgresrepo.NewTripRepositoryWithDB(tx)
			userRoles := repository.NewUserRoleRepository(tx)
			companyMemberships :=
				postgresrepo.NewCompanyMembershipRepository(tx)

			driver, err := drivers.GetByIDForUpdate(ctx, id)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrDriverNotFound
				}

				return fmt.Errorf("get driver for archive: %w", err)
			}

			roles, err := userRoles.GetUserRoles(ctx, userID)
			if err != nil {
				return fmt.Errorf("get user roles: %w", err)
			}

			systemAdmin := false
			for _, role := range roles {
				if role == "SYSTEM_ADMIN" {
					systemAdmin = true
					break
				}
			}

			if !systemAdmin {
				member, err := companyMemberships.Exists(
					ctx,
					userID,
					driver.CompanyID,
				)
				if err != nil {
					return fmt.Errorf("check company membership: %w", err)
				}

				if !member {
					return ErrDriverNotFound
				}
			}

			driverPresence, err := presence.GetByDriverIDForUpdate(
				ctx,
				driver.UserID,
			)
			switch {
			case err == nil:
				if driverPresence.IsOnline {
					return ErrDriverOperationallyActive
				}

			case errors.Is(err, repository.ErrNotFound):
				// A presence row is not required for an offline driver.

			default:
				return fmt.Errorf("lock driver presence for archive: %w", err)
			}

			_, err = assignments.GetActiveByDriver(
				ctx,
				driver.UserID,
			)
			switch {
			case err == nil:
				return ErrDriverHasActiveAssignment

			case errors.Is(err, repository.ErrNotFound):
				// No active assignment.

			default:
				return fmt.Errorf("check active driver assignment: %w", err)
			}

			_, err = trips.GetActiveByDriverID(
				ctx,
				driver.UserID,
			)
			switch {
			case err == nil:
				return ErrDriverHasActiveTrip

			case errors.Is(err, repository.ErrNotFound):
				// No active trip.

			default:
				return fmt.Errorf("check active driver trip: %w", err)
			}

			if err := drivers.Delete(ctx, driver.ID); err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrDriverNotFound
				}

				return fmt.Errorf("archive driver: %w", err)
			}

			return nil
		},
	)
}
