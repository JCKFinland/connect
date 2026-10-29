package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Create creates a new trip after applying basic business validation.
func (s *tripService) Create(
	ctx context.Context,
	trip *models.Trip,
) error {
	if trip == nil {
		return fmt.Errorf("trip is required")
	}

	if trip.RideRequestID == "" {
		return fmt.Errorf("ride request ID is required")
	}

	if trip.CustomerID == "" {
		return fmt.Errorf("customer ID is required")
	}

	if trip.DriverID == "" {
		return fmt.Errorf("driver ID is required")
	}

	if trip.VehicleID == "" {
		return fmt.Errorf("vehicle ID is required")
	}

	if trip.CompanyID == "" {
		return fmt.Errorf("company ID is required")
	}

	if trip.BranchID == "" {
		return fmt.Errorf("branch ID is required")
	}

	if trip.FleetID == "" {
		return fmt.Errorf("fleet ID is required")
	}

	if trip.ID == "" {
		trip.ID = uuid.NewString()
	}

	// A newly created trip always enters the canonical initial lifecycle
	// state. Progressed lifecycle evidence must be established only by
	// dedicated lifecycle mutation paths.
	now := time.Now().UTC()

	trip.Status = StatusAssigned
	trip.AssignedAt = now

	trip.DriverArrivedAt = nil
	trip.StartedAt = nil
	trip.CompletedAt = nil
	trip.CancelledAt = nil

	trip.ActualDistanceKM = nil
	trip.ActualDurationMinutes = nil
	trip.ActualDistanceMeters = nil
	trip.ActualDurationSeconds = nil

	trip.CancelledBy = nil
	trip.CancellationReason = nil

	trip.IsActive = true

	if trip.CreatedAt.IsZero() {
		trip.CreatedAt = now
	}

	if trip.UpdatedAt.IsZero() {
		trip.UpdatedAt = now
	}

	return s.repo.Create(ctx, trip)
}

// CreateAuthorized creates a trip only when the authenticated user
// has operational trip-management privileges.
func (s *tripService) CreateAuthorized(
	ctx context.Context,
	trip *models.Trip,
	userID string,
) error {

	if trip == nil {
		return fmt.Errorf("trip is required")
	}
	if trip.CompanyID == "" {
		return fmt.Errorf("company ID is required")
	}

	if err := s.authorizeOperationalMutation(
		ctx,
		userID,
	); err != nil {
		return err
	}

	roles, err := s.userRoles.GetUserRoles(
		ctx,
		userID,
	)
	if err != nil {
		return fmt.Errorf(
			"get user roles for trip creation: %w",
			err,
		)
	}

	if s.db == nil {
		return fmt.Errorf("trip database is not configured")
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			allowed, err := s.hasOperationalCompanyAuthorityForUpdate(
				ctx,
				tx,
				userID,
				trip.CompanyID,
				roles,
			)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrTripMutationAccessDenied
			}

			transactionService := *s
			transactionService.repo = postgresrepo.NewTripRepositoryWithDB(tx)

			return transactionService.Create(ctx, trip)
		},
	)
}
