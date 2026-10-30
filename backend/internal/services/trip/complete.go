package trip

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/fare"
	"github.com/JCKFinland/connect/backend/internal/services/tripmeter"
	"github.com/jackc/pgx/v5"
)

// CompleteTrip atomically finalizes operational trip measurements,
// fare calculation, lifecycle state, audit event, and driver release.
// ErrTripCompletionValidation identifies a client-correctable
// completion precondition failure.
var ErrTripCompletionValidation = errors.New(
	"trip completion precondition failed",
)

func (s *tripService) CompleteTrip(
	ctx context.Context,
	id string,
	performedByUserID string,
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

			tripEvents :=
				postgresrepo.NewTripEventRepositoryWithDB(tx)

			tripFares :=
				postgresrepo.NewTripFareRepositoryWithDB(tx)
			farePricingProfiles :=
				postgresrepo.NewFarePricingProfileRepositoryWithDB(tx)
			tripLocations :=
				postgresrepo.NewTripLocationRepositoryWithDB(tx)
			tripMeterMeasurements :=
				postgresrepo.NewTripMeterMeasurementRepositoryWithDB(tx)

				// -------------------------------------------------
				// Lock authoritative trip row.
				// -------------------------------------------------

			// Resolve assignment before acquiring permanent-driver locks.
			// Revalidate after locking the authoritative trip row.
			unlockedTrip, err := trips.GetByID(ctx, id)
			if err != nil {
				return fmt.Errorf("read trip before terminal locks: %w", err)
			}
			terminalLocks, err := lockTerminalTripDriver(
				ctx, tx, unlockedTrip.DriverID,
			)
			if err != nil {
				return err
			}
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

			if currentTrip.DriverID != unlockedTrip.DriverID {
				return fmt.Errorf(
					"trip driver assignment changed during terminal locking",
				)
			}

			allowed, err := s.canUpdateTripStatusAuthorizedForUpdate(
				ctx,
				tx, roles,
				performedByUserID,
				currentTrip,
			)
			if err != nil {
				return err
			}
			if !allowed {
				return ErrTripStatusAccessDenied
			}

			// Completion is intentionally restricted to the
			// operational IN_PROGRESS state.
			if currentTrip.Status != StatusInProgress {
				return fmt.Errorf(
					"%w: trip must be IN_PROGRESS before completion: current status %s",
					ErrTripCompletionValidation,
					currentTrip.Status,
				)
			}

			// Pricing mode belongs to the originating ride request.
			// Resolve it inside the locked completion transaction.
			rideRequests :=
				postgresrepo.NewRideRequestRepositoryWithDB(tx)

			rideRequest, err := rideRequests.GetByID(
				ctx,
				currentTrip.RideRequestID,
			)
			if err != nil {
				return fmt.Errorf(
					"get originating ride request for completion: %w",
					err,
				)
			}

			if rideRequest.CustomerID != currentTrip.CustomerID {
				return fmt.Errorf(
					"trip customer does not match originating ride request",
				)
			}

			if rideRequest.MatchingMode != models.RideMatchingModeStandard &&
				rideRequest.MatchingMode != models.RideMatchingModeFlex {
				return fmt.Errorf(
					"unsupported trip matching mode: %s",
					rideRequest.MatchingMode,
				)
			}

			isFlex := rideRequest.MatchingMode ==
				models.RideMatchingModeFlex

			if currentTrip.PricingProfileID == nil ||
				*currentTrip.PricingProfileID == "" {
				return fmt.Errorf(
					"trip has no frozen pricing profile",
				)
			}

			pricingProfile, err :=
				farePricingProfiles.GetByID(
					ctx,
					*currentTrip.PricingProfileID,
				)
			if err != nil {
				return fmt.Errorf(
					"get frozen trip pricing profile: %w",
					err,
				)
			}

			if pricingProfile == nil {
				return fmt.Errorf(
					"frozen trip pricing profile was not found",
				)
			}

			if pricingProfile.ID != *currentTrip.PricingProfileID {
				return fmt.Errorf(
					"frozen trip pricing profile mismatch",
				)
			}

			if currentTrip.ServiceCategoryID == nil ||
				*currentTrip.ServiceCategoryID == "" {
				if !isFlex {
					return fmt.Errorf(
						"trip has no frozen service category",
					)
				}
			} else if pricingProfile.ServiceCategoryID !=
				*currentTrip.ServiceCategoryID {
				return fmt.Errorf(
					"frozen pricing profile service category does not match trip service category",
				)
			}

			if pricingProfile.CompanyID != currentTrip.CompanyID {
				return fmt.Errorf(
					"frozen pricing profile company does not match trip company",
				)
			}

			// -------------------------------------------------
			// Calculate authoritative operational measurements
			// from persisted trip GPS/location evidence.
			//
			// The repository is transaction-bound so the evidence
			// used for finalization is read within the same
			// transaction that locks and completes the trip.
			// -------------------------------------------------

			tripMeterService := tripmeter.NewService(
				tripmeter.Dependencies{
					TripLocations: tripLocations,
				},
			)

			measurement, err :=
				tripMeterService.MeasureTrip(
					ctx,
					currentTrip.ID,
				)
			if err != nil {
				return fmt.Errorf(
					"measure trip from location evidence: %w",
					err,
				)
			}

			if measurement.AcceptedSamples < 2 {
				return fmt.Errorf(
					"%w: insufficient trip location evidence: accepted samples %d",
					ErrTripCompletionValidation,
					measurement.AcceptedSamples,
				)
			}

			// -------------------------------------------------
			// Persist immutable authoritative meter snapshot.
			//
			// This records exactly which measurement result was
			// used to finalize the trip and calculate its fare.
			//
			// Because this repository is transaction-bound, the
			// snapshot rolls back if any later completion step
			// fails.
			// -------------------------------------------------

			meterSnapshot := &models.TripMeterMeasurement{
				TripID: currentTrip.ID,

				MeasurementSource: tripmeter.MeasurementSourceGPS,
				AlgorithmVersion:  tripmeter.AlgorithmVersionGPSV1,

				DistanceMeters:         measurement.DistanceMeters,
				DurationSeconds:        measurement.DurationSeconds,
				WaitingDurationSeconds: measurement.WaitingDurationSeconds,

				AcceptedSamples:  measurement.AcceptedSamples,
				RejectedSamples:  measurement.RejectedSamples,
				RejectedSegments: measurement.RejectedSegments,

				MeasuredAt: time.Now().UTC(),
			}

			if err := tripMeterMeasurements.Create(
				ctx,
				meterSnapshot,
			); err != nil {
				return fmt.Errorf(
					"persist trip meter measurement: %w",
					err,
				)
			}

			// -------------------------------------------------
			// Calculate the immutable fare snapshot before any
			// trip lifecycle state is changed.
			// -------------------------------------------------

			var calculatedFare *models.TripFare

			if isFlex {
				// FLEX is a fixed contractual fare. GPS measurements
				// remain authoritative operational evidence, but must
				// never recalculate the negotiated passenger price.
				contract, err :=
					postgresrepo.ResolveConfirmedFlexFareContract(
						ctx,
						tx,
						currentTrip.ID,
					)
				if err != nil {
					return fmt.Errorf(
						"resolve authorized FLEX fare contract: %w",
						err,
					)
				}

				calculatedFare, err = fare.BuildFlexFixedFare(
					fare.FlexFixedFareInput{
						TripID:           currentTrip.ID,
						AmountCents:      contract.AmountCents,
						Currency:         contract.Currency,
						PricingProfileID: contract.PricingProfileID,
						PricingVersion:   contract.PricingVersion,
						CalculatedAt:     time.Now().UTC(),
					},
				)
				if err != nil {
					return fmt.Errorf(
						"build authorized FLEX fixed fare: %w",
						err,
					)
				}
			} else {
				// STANDARD retains the existing GPS-metered
				// calculation and frozen pricing inputs.
				calculatedFare, err = s.fareCalculator.Calculate(
					fare.CalculationInput{
						TripID: currentTrip.ID,

						DistanceMeters:  measurement.DistanceMeters,
						DurationSeconds: measurement.DurationSeconds,
						WaitingSeconds:  measurement.WaitingDurationSeconds,

						Pricing: fare.PricingSnapshot{
							BaseFare:             pricingProfile.BaseFare,
							DistanceRatePerKM:    pricingProfile.DistanceRatePerKM,
							TimeRatePerMinute:    pricingProfile.TimeRatePerMinute,
							WaitingRatePerMinute: pricingProfile.WaitingRatePerMinute,
							BookingFee:           pricingProfile.BookingFee,
							SurgeMultiplier:      pricingProfile.SurgeMultiplier,
							Currency:             pricingProfile.Currency,
							PricingVersion:       pricingProfile.Version,
						},
					},
				)
				if err != nil {
					return fmt.Errorf(
						"calculate trip fare: %w",
						err,
					)
				}

				calculatedFare.PricingProfileID =
					currentTrip.PricingProfileID
			}

			// -------------------------------------------------
			// Persist authoritative operational meter values.
			// -------------------------------------------------

			if err := trips.UpdateActualMetrics(
				ctx,
				currentTrip.ID,
				measurement.DistanceMeters,
				measurement.DurationSeconds,
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
					"previous_status":      currentTrip.Status,
					"new_status":           StatusCompleted,
					"fare_id":              calculatedFare.ID,
					"meter_measurement_id": meterSnapshot.ID,
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

			if err := releaseTerminalTripDriver(
				ctx,
				tx,
				terminalLocks,
				currentTrip.ID,
			); err != nil {
				return err
			}

			// -------------------------------------------------
			// Atomically create the durable FLEX settlement.
			//
			// This uses the current completion transaction.
			// No Stripe request occurs during trip completion.
			// -------------------------------------------------

			if isFlex {
				if _, err := postgresrepo.CreateFlexSettlementFromCompletedTripTx(
					ctx,
					tx,
					currentTrip.ID,
				); err != nil {
					return fmt.Errorf(
						"create completed FLEX trip settlement: %w",
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
