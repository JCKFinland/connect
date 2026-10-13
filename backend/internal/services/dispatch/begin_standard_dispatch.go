package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// BeginStandardDispatch durably records that standard dispatch has started.
//
// The marker is committed in its own transaction so a later unsuccessful
// dispatch attempt cannot erase the historical evidence.
//
// Every competing matching-mode transition must acquire the same advisory
// lock before locking the ride-request row.
func (s *Service) BeginStandardDispatch(
	ctx context.Context,
	rideRequestID string,
) error {
	if s == nil || s.db == nil {
		return errors.New("dispatch database is not configured")
	}

	if rideRequestID == "" {
		return errors.New("ride request ID is required")
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
				return fmt.Errorf(
					"lock standard dispatch request: %w",
					err,
				)
			}

			requests := postgresrepo.NewRideRequestRepositoryWithDB(tx)

			request, err := requests.GetByIDForUpdate(
				ctx,
				rideRequestID,
			)
			if err != nil {
				return fmt.Errorf(
					"load standard dispatch request: %w",
					err,
				)
			}

			if request.MatchingMode != models.RideMatchingModeStandard {
				return fmt.Errorf(
					"standard dispatch requires STANDARD matching mode",
				)
			}

			if request.Status != rideRequestStatusPending {
				return fmt.Errorf(
					"standard dispatch requires PENDING ride request",
				)
			}

			now := time.Now().UTC()

			if request.ExpiresAt != nil &&
				!now.Before(request.ExpiresAt.UTC()) {
				return ErrRideRequestExpired
			}

			const query = `
				UPDATE ride_requests
				SET standard_dispatch_started_at = NOW()
				WHERE id = $1
				  AND matching_mode = 'STANDARD'
				  AND status = 'PENDING'
				  AND standard_dispatch_started_at IS NULL
				  AND (
					  expires_at IS NULL
					  OR expires_at > NOW()
				  )
			`

			result, err := tx.Exec(
				ctx,
				query,
				rideRequestID,
			)
			if err != nil {
				return fmt.Errorf(
					"record standard dispatch start: %w",
					err,
				)
			}

			if result.RowsAffected() == 0 {
				var alreadyStarted bool

				if err := tx.QueryRow(
					ctx,
					`
						SELECT standard_dispatch_started_at IS NOT NULL
						FROM ride_requests
						WHERE id = $1
					`,
					rideRequestID,
				).Scan(&alreadyStarted); err != nil {
					return fmt.Errorf(
						"verify standard dispatch marker: %w",
						err,
					)
				}

				if !alreadyStarted {
					return errors.New(
						"standard dispatch marker was not persisted",
					)
				}
			}

			return nil
		},
	)
}
