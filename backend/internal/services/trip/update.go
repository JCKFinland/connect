package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Update updates editable trip information.
func (s *tripService) Update(
	ctx context.Context,
	trip *models.Trip,
) error {
	if trip == nil {
		return fmt.Errorf("trip is required")
	}

	if trip.ID == "" {
		return fmt.Errorf("trip ID is required")
	}

	// Protected fields such as driver, vehicle, company,
	// branch, fleet, customer, and status are intentionally
	// not changed by the repository Update operation.
	return s.repo.Update(ctx, trip)
}

// UpdateAuthorized updates editable trip information only when the
// authenticated user has operational trip-management privileges.
func (s *tripService) UpdateAuthorized(
	ctx context.Context,
	trip *models.Trip,
	userID string,
) error {

	if trip == nil {
		return fmt.Errorf("trip is required")
	}

	if trip.ID == "" {
		return fmt.Errorf("trip ID is required")
	}

	if err := s.authorizeOperationalMutation(
		ctx,
		userID,
	); err != nil {
		return err
	}

	// Authorize against the persisted company while holding the trip
	// row lock. Update through the same transaction.
	if s.db == nil {
		return fmt.Errorf("trip database is not configured")
	}

	roles, err := s.userRoles.GetUserRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user roles for trip update: %w", err)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			trips := postgresrepo.NewTripRepositoryWithDB(tx)

			currentTrip, err := trips.GetByIDForUpdate(ctx, trip.ID)
			if err != nil {
				return fmt.Errorf("get trip for update: %w", err)
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

			if err := trips.Update(ctx, trip); err != nil {
				return fmt.Errorf("update authorized trip: %w", err)
			}

			return nil
		},
	)
}
