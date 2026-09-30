package presence

import (
	"context"
	"errors"
	"fmt"
	"time"

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

	complianceRejected := false

	err := postgresrepo.RunInTransaction(
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

			current, err :=
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
			// Revalidate regulatory compliance for idle online states.
			//
			// BUSY drivers must keep sending telemetry for an active trip
			// even if regulatory eligibility changes mid-trip. Dispatch and
			// offer acceptance independently prevent new work from being
			// assigned to a non-compliant driver.
			//
			// AVAILABLE and BREAK are idle online states. If compliance has
			// lapsed, remove the driver from the online pool while the presence
			// lifecycle row remains locked.
			if current != nil &&
				current.IsOnline &&
				(current.AvailabilityStatus == StatusAvailable ||
					current.AvailabilityStatus == StatusBreak) {

				if s.drivers == nil {
					return errors.New(
						"driver repository is not configured",
					)
				}

				if s.compliance == nil {
					return errors.New(
						"driver compliance service is not configured",
					)
				}

				driver, err := s.getDriverByUserID(
					ctx,
					req.UserID,
				)
				if err != nil {
					return err
				}

				eligible, err := s.compliance.IsEligible(
					ctx,
					driver.ID,
					time.Now().UTC(),
				)
				if err != nil {
					return fmt.Errorf(
						"evaluate driver compliance before heartbeat: %w",
						err,
					)
				}

				if !eligible {
					transitioned, err :=
						presenceRepo.UpdateAvailabilityIfIdle(
							ctx,
							req.UserID,
							StatusOffline,
							false,
						)

					if err != nil {
						return fmt.Errorf(
							"mark non-compliant driver offline: %w",
							err,
						)
					}

					if transitioned {
						complianceRejected = true
						return nil
					}
				}
			}

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
	if err != nil {
		return err
	}

	if complianceRejected {
		return ErrDriverComplianceRequired
	}

	return nil
}
