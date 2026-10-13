package ride_request

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var (
	ErrFlexActivationAccessDenied = errors.New("flex activation access denied")
	ErrFlexActivationNotEligible  = errors.New("ride request is not eligible for flex activation")
)

// ActivateFlexAuthorized transitions an eligible customer-owned ride request
// from STANDARD matching to FLEX matching.
//
// The advisory lock serializes this transition with STANDARD dispatch.
// The ride-request row is locked only after acquiring that advisory lock.
func (s *Service) ActivateFlexAuthorized(
	ctx context.Context,
	rideRequestID string,
	userID string,
) error {
	if s == nil || s.db == nil {
		return errors.New("ride-request database is not configured")
	}

	if rideRequestID == "" {
		return errors.New("ride request ID is required")
	}

	if userID == "" {
		return errors.New("authenticated user ID is required")
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"ride-dispatch:"+rideRequestID,
			); err != nil {
				return fmt.Errorf("lock flex activation: %w", err)
			}

			requests := postgresrepo.NewRideRequestRepositoryWithDB(tx)

			request, err := requests.GetByIDForUpdate(
				ctx,
				rideRequestID,
			)
			if err != nil {
				return fmt.Errorf("load flex activation request: %w", err)
			}

			// Privileged read access does not grant activation permission.
			if request.CustomerID != userID {
				return ErrFlexActivationAccessDenied
			}

			// Ownership is necessary but insufficient: only accounts
			// currently assigned the CUSTOMER role may activate FLEX.
			if s.userRoles == nil {
				return errors.New("ride-request role repository is not configured")
			}

			roles, err := s.getUserRoles(ctx, userID)
			if err != nil {
				return fmt.Errorf("authorize flex activation: %w", err)
			}

			isCustomer := false
			for _, role := range roles {
				if role == "CUSTOMER" {
					isCustomer = true
					break
				}
			}

			if !isCustomer {
				return ErrFlexActivationAccessDenied
			}

			if request.Status != "PENDING" ||
				request.MatchingMode != models.RideMatchingModeStandard {
				return ErrFlexActivationNotEligible
			}

			if request.ExpiresAt != nil &&
				!time.Now().UTC().Before(request.ExpiresAt.UTC()) {
				return ErrFlexActivationNotEligible
			}

			// The durable dispatch marker is checked by the UPDATE itself.
			// The database trigger independently protects marked requests.
			result, err := tx.Exec(
				ctx,
				`
				UPDATE ride_requests
				SET matching_mode = 'FLEX'
				WHERE id = $1
				  AND customer_id = $2
				  AND status = 'PENDING'
				  AND matching_mode = 'STANDARD'
				  AND standard_dispatch_started_at IS NULL
				  AND (
					expires_at IS NULL
					OR expires_at > NOW()
				  )
				`,
				rideRequestID,
				userID,
			)
			if err != nil {
				return fmt.Errorf("activate flex matching: %w", err)
			}

			if result.RowsAffected() != 1 {
				return ErrFlexActivationNotEligible
			}

			return nil
		},
	)
}
