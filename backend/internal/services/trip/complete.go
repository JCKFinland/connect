package trip

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/fare"
	"github.com/jackc/pgx/v5"
)

// CompleteTrip atomically finalizes operational trip measurements,
// fare calculation, lifecycle state, audit event, and driver release.
func (s *tripService) CompleteTrip(
	ctx context.Context,
	id string,
	performedByUserID string,
	input CompleteTripInput,
) (*models.TripFare, error) {
	if id == "" {
		return nil, fmt.Errorf(
			"trip ID is required",
		)
	}

	if performedByUserID == "" {
		return nil, fmt.Errorf(
			"performed by user ID is required",
		)
	}

	if s.db == nil {
		return nil, fmt.Errorf(
			"trip database is not configured",
		)
	}

	if s.fareCalculator == nil {
		return nil, fmt.Errorf(
			"fare calculator is not configured",
		)
	}

	if input.ActualDistanceMeters < 0 {
		return nil, fmt.Errorf(
			"actual distance cannot be negative",
		)
	}

	if input.ActualDurationSeconds < 0 {
		return nil, fmt.Errorf(
			"actual duration cannot be negative",
		)
	}

	if input.WaitingDurationSeconds < 0 {
		return nil, fmt.Errorf(
			"waiting duration cannot be negative",
		)
	}

	roles, err := s.userRoles.GetUserRoles(
		ctx,
		performedByUserID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get user roles for trip completion: %w",
			err,
		)
	}

	var completedFare *models.TripFare

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			trips :=
				postgresrepo.NewTripRepositoryWithDB(tx)

			presence :=
				postgresrepo.NewDriverPresenceRepositoryWithDB(tx)

			tripEvents :=
				postgresrepo.NewTripEventRepositoryWithDB(tx)

			tripFares :=
				postgresrepo.NewTripFareRepositoryWithDB(tx)

			// -------------------------------------------------
			// Lock authoritative trip row.
			// -------------------------------------------------

			currentTrip, err :=
				trips.GetByIDForUpdate(
					ctx,
					id,
				)
			if err != nil {
				return fmt.Errorf(
					"get trip for completion: %w",
					err,
				)
			}

			if !canUpdateTripStatus(
				roles,
				performedByUserID,
				currentTrip,
			) {
				return ErrTripStatusAccessDenied
			}

			// Completion is intentionally restricted to the
			// operational IN_PROGRESS state.
			if currentTrip.Status != StatusInProgress {
				return fmt.Errorf(
					"trip must be IN_PROGRESS before completion: current status %s",
					currentTrip.Status,
				)
			}

			// -------------------------------------------------
			// Calculate the immutable fare snapshot before any
			// trip lifecycle state is changed.
			// -------------------------------------------------

			calculatedFare, err :=
				s.fareCalculator.Calculate(
					fare.CalculationInput{
						TripID: currentTrip.ID,

						DistanceMeters: input.ActualDistanceMeters,

						DurationSeconds: input.ActualDurationSeconds,

						WaitingSeconds: input.WaitingDurationSeconds,

						Pricing: fare.PricingSnapshot{
							BaseFare: input.BaseFare,

							DistanceRatePerKM: input.DistanceRatePerKM,

							TimeRatePerMinute: input.TimeRatePerMinute,

							WaitingRatePerMinute: input.WaitingRatePerMinute,

							BookingFee: input.BookingFee,

							SurgeMultiplier: input.SurgeMultiplier,

							DiscountAmount: input.DiscountAmount,

							TaxAmount: input.TaxAmount,

							TollAmount: input.TollAmount,

							ParkingAmount: input.ParkingAmount,

							Currency: input.Currency,

							PricingVersion: input.PricingVersion,
						},
					},
				)
			if err != nil {
				return fmt.Errorf(
					"calculate trip fare: %w",
					err,
				)
			}

			// -------------------------------------------------
			// Persist authoritative operational meter values.
			// -------------------------------------------------

			if err := trips.UpdateActualMetrics(
				ctx,
				currentTrip.ID,
				input.ActualDistanceMeters,
				input.ActualDurationSeconds,
			); err != nil {
				return fmt.Errorf(
					"persist trip completion metrics: %w",
					err,
				)
			}

			// -------------------------------------------------
			// Persist exactly one authoritative fare.
			//
			// UNIQUE(trip_id) provides database-level protection
			// against multiple finalized fares.
			// -------------------------------------------------

			if err := tripFares.Create(
				ctx,
				calculatedFare,
			); err != nil {
				return fmt.Errorf(
					"persist trip fare: %w",
					err,
				)
			}

			// -------------------------------------------------
			// Only after fare persistence succeeds may the trip
			// become COMPLETED.
			// -------------------------------------------------

			if err := trips.UpdateStatus(
				ctx,
				currentTrip.ID,
				StatusCompleted,
			); err != nil {
				return fmt.Errorf(
					"mark trip completed: %w",
					err,
				)
			}

			// -------------------------------------------------
			// Immutable lifecycle audit event.
			// -------------------------------------------------

			metadata, err := json.Marshal(
				map[string]string{
					"previous_status": currentTrip.Status,
					"new_status":      StatusCompleted,
					"fare_id":         calculatedFare.ID,
				},
			)
			if err != nil {
				return fmt.Errorf(
					"marshal trip completion event metadata: %w",
					err,
				)
			}

			event := &models.TripEvent{
				TripID: currentTrip.ID,

				EventType: EventTripCompleted,

				Metadata: metadata,

				OccurredAt: time.Now().UTC(),
			}

			event.PerformedByUserID =
				&performedByUserID

			if err := tripEvents.Create(
				ctx,
				event,
			); err != nil {
				return fmt.Errorf(
					"create trip completion event: %w",
					err,
				)
			}

			// -------------------------------------------------
			// Release operational driver.
			// -------------------------------------------------

			if currentTrip.DriverID != "" {
				if err := presence.UpdateAvailability(
					ctx,
					currentTrip.DriverID,
					driverAvailabilityAvailable,
					true,
				); err != nil {
					return fmt.Errorf(
						"release driver after completed trip: %w",
						err,
					)
				}
			}

			completedFare = calculatedFare

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return completedFare, nil
}
