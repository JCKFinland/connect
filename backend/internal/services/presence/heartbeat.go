package presence

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var ErrDriverHeartbeatUnavailable = errors.New(
	"driver heartbeat is unavailable while driver is offline",
)

var (
	ErrInvalidLatitude = errors.New(
		"latitude must be between -90 and 90",
	)

	ErrInvalidLongitude = errors.New(
		"longitude must be between -180 and 180",
	)

	ErrInvalidHeading = errors.New(
		"heading must be between 0 and 360",
	)

	ErrInvalidSpeed = errors.New(
		"speed must be zero or greater",
	)

	ErrInvalidAccuracy = errors.New(
		"accuracy must be zero or greater",
	)
)

func (s *Service) Heartbeat(
	ctx context.Context,
	req HeartbeatRequest,
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
	// 1. Validate telemetry before touching persistence.
	//
	// PostgreSQL CHECK constraints remain the final integrity
	// backstop, but malformed GPS data should be rejected at
	// the service boundary.
	// ---------------------------------------------------------

	if req.Latitude < -90 ||
		req.Latitude > 90 {

		return ErrInvalidLatitude
	}

	if req.Longitude < -180 ||
		req.Longitude > 180 {

		return ErrInvalidLongitude
	}

	if req.Heading < 0 ||
		req.Heading > 360 {

		return ErrInvalidHeading
	}

	if req.Speed < 0 {
		return ErrInvalidSpeed
	}

	if req.Accuracy < 0 {
		return ErrInvalidAccuracy
	}

	// ---------------------------------------------------------
	// 2. Lock and update the driver's heartbeat atomically.
	//
	// The driver_presence row is the lifecycle serialization
	// point shared by online/offline, availability, assignment,
	// and dispatch operations.
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
			// Lock presence first.
			//
			// This distinguishes:
			//
			//   missing presence
			//       -> ErrDriverNotFound
			//
			//   existing but offline/inactive presence
			//       -> ErrDriverHeartbeatUnavailable
			//
			// It also prevents the row from disappearing or changing
			// lifecycle state between existence checking and heartbeat.
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
					"lock driver presence before heartbeat: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// Update live telemetry only for online operational states.
			//
			// Valid:
			//   AVAILABLE
			//   BUSY
			//   BREAK
			//
			// Rejected:
			//   OFFLINE
			//   OFF_DUTY
			//   SUSPENDED
			// ---------------------------------------------------------

			updated, err :=
				presenceRepo.UpdateHeartbeatIfOnline(
					ctx,
					req.UserID,
					req.Latitude,
					req.Longitude,
					req.Heading,
					req.Speed,
					req.Accuracy,
				)

			if err != nil {
				return fmt.Errorf(
					"update driver heartbeat: %w",
					err,
				)
			}

			if !updated {
				return ErrDriverHeartbeatUnavailable
			}

			return nil
		},
	)
}
