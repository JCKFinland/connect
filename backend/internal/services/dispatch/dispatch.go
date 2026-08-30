package dispatch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/pricing"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNoAvailableDrivers = errors.New("no available drivers found")
)

const (
	rideRequestStatusPending  = "PENDING"
	rideRequestStatusAccepted = "ACCEPTED"

	tripStatusAssigned = "ASSIGNED"

	driverStatusBusy = "BUSY"
)

type rankedCandidate struct {
	presence   *models.DriverPresence
	assignment *models.DriverAssignment
	distanceKM float64
}

// DispatchRide finds the first fully eligible driver for a pending ride request
// and atomically creates the trip, marks the driver busy, and accepts the request.
func (s *Service) DispatchRide(
	ctx context.Context,
	rideRequestID string,
) (*models.Trip, error) {

	if s == nil {
		return nil, errors.New("dispatch service is required")
	}

	if s.db == nil {
		return nil, errors.New("dispatch database is not configured")
	}

	if s.cfg == nil {
		return nil, errors.New("dispatch configuration is not configured")
	}

	if s.cfg.Presence.HeartbeatTimeout <= 0 {
		return nil, errors.New(
			"presence heartbeat timeout must be greater than zero",
		)
	}

	if rideRequestID == "" {
		return nil, errors.New("ride request ID is required")
	}

	var dispatchedTrip *models.Trip

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			// All repositories below use the same PostgreSQL transaction.
			rideRequests := postgresrepo.NewRideRequestRepositoryWithDB(tx)
			assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)
			presence := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)
			trips := postgresrepo.NewTripRepositoryWithDB(tx)
			vehicles := postgresrepo.NewVehicleRepositoryWithDB(tx)

			farePricingProfiles :=
				postgresrepo.NewFarePricingProfileRepositoryWithDB(tx)

			pricingService := pricing.NewService(
				pricing.Dependencies{
					FarePricingProfiles: farePricingProfiles,
				},
			)

			// ---------------------------------------------------------
			// 1. Load and validate ride request
			// ---------------------------------------------------------

			request, err := rideRequests.GetByIDForUpdate(
				ctx,
				rideRequestID,
			)
			if err != nil {
				return fmt.Errorf(
					"get ride request: %w",
					err,
				)
			}

			if request.Status != rideRequestStatusPending {
				return fmt.Errorf(
					"ride request must be %s before dispatch",
					rideRequestStatusPending,
				)
			}

			// ---------------------------------------------------------
			// 2. Get currently online + AVAILABLE candidates
			// ---------------------------------------------------------

			availableDrivers, err := presence.ListAllAvailable(ctx)
			if err != nil {
				return fmt.Errorf(
					"list available drivers: %w",
					err,
				)
			}

			if len(availableDrivers) == 0 {
				return ErrNoAvailableDrivers
			}

			// ---------------------------------------------------------
			// 3. Evaluate and rank fully eligible candidates
			//
			// A candidate must have:
			// - online + AVAILABLE presence
			// - a recent heartbeat
			// - an active driver/vehicle assignment
			// - an existing assigned vehicle
			// - a vehicle compatible with the requested ride type
			// ---------------------------------------------------------

			now := time.Now().UTC()

			candidates := make(
				[]rankedCandidate,
				0,
				len(availableDrivers),
			)

			// ---------------------------------------------------------
			// Evaluate all currently discoverable candidates.
			// ---------------------------------------------------------

			for _, candidate := range availableDrivers {

				if candidate == nil {
					continue
				}

				if candidate.DriverID == "" {
					continue
				}

				if candidate.LastHeartbeatAt == nil {
					continue
				}

				heartbeatAge := now.Sub(
					candidate.LastHeartbeatAt.UTC(),
				)

				// Ignore stale or invalid future heartbeat timestamps.
				if heartbeatAge < 0 ||
					heartbeatAge > s.cfg.Presence.HeartbeatTimeout {
					continue
				}

				// A dispatch candidate must have valid location data.
				if candidate.Latitude == nil ||
					candidate.Longitude == nil {
					continue
				}

				candidateAssignment, err := assignments.GetActiveByDriver(
					ctx,
					candidate.DriverID,
				)
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"get candidate driver assignment: %w",
						err,
					)
				}

				if candidateAssignment == nil ||
					candidateAssignment.VehicleID == "" {
					continue
				}

				vehicle, err := vehicles.GetByID(
					ctx,
					candidateAssignment.VehicleID,
				)
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"get candidate vehicle: %w",
						err,
					)
				}

				if vehicle == nil || !vehicle.IsActive {
					continue
				}

				if !isVehicleEligible(
					request.RequestedVehicleType,
					vehicle.VehicleType,
				) {
					continue
				}

				candidateDistanceKM := distanceKM(
					*candidate.Latitude,
					*candidate.Longitude,
					request.PickupLatitude,
					request.PickupLongitude,
				)

				candidates = append(
					candidates,
					rankedCandidate{
						presence:   candidate,
						assignment: candidateAssignment,
						distanceKM: candidateDistanceKM,
					},
				)
			}

			if len(candidates) == 0 {
				return ErrNoAvailableDrivers
			}

			// ---------------------------------------------------------
			// Rank nearest candidate first.
			// ---------------------------------------------------------

			sort.SliceStable(
				candidates,
				func(i, j int) bool {
					return candidates[i].distanceKM <
						candidates[j].distanceKM
				},
			)

			// ---------------------------------------------------------
			// Claim the nearest driver that is still available.
			//
			// SKIP LOCKED allows concurrent dispatch transactions to
			// move on to the next candidate instead of blocking or
			// double-assigning the same driver.
			// ---------------------------------------------------------

			var selected *models.DriverPresence
			var selectedAssignment *models.DriverAssignment

			for _, candidate := range candidates {

				if s.beforeClaimCandidate != nil {
					s.beforeClaimCandidate(
						candidate.presence.DriverID,
					)
				}

				lockedDriver, err := presence.GetAvailableByDriverIDForUpdate(
					ctx,
					candidate.presence.DriverID,
				)

				if errors.Is(err, repository.ErrNotFound) {
					// Another dispatch may have claimed or locked this driver.
					// Try the next-nearest eligible candidate.
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"lock candidate driver: %w",
						err,
					)
				}

				// Re-check heartbeat freshness after acquiring the lock.
				if lockedDriver.LastHeartbeatAt == nil {
					continue
				}

				lockedHeartbeatAge := now.Sub(
					lockedDriver.LastHeartbeatAt.UTC(),
				)

				if lockedHeartbeatAge < 0 ||
					lockedHeartbeatAge > s.cfg.Presence.HeartbeatTimeout {
					continue
				}

				lockedAssignment, err := assignments.GetActiveByDriver(
					ctx,
					lockedDriver.DriverID,
				)
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"recheck locked driver assignment: %w",
						err,
					)
				}

				if lockedAssignment == nil ||
					lockedAssignment.VehicleID == "" {
					continue
				}

				lockedVehicle, err := vehicles.GetByID(
					ctx,
					lockedAssignment.VehicleID,
				)
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"recheck locked driver vehicle: %w",
						err,
					)
				}

				if lockedVehicle == nil ||
					!lockedVehicle.IsActive {
					continue
				}

				if !isVehicleEligible(
					request.RequestedVehicleType,
					lockedVehicle.VehicleType,
				) {
					continue
				}

				selected = lockedDriver
				selectedAssignment = lockedAssignment

				break
			}

			if selected == nil || selectedAssignment == nil {
				return ErrNoAvailableDrivers
			}

			if request.ServiceCategoryID == nil ||
				*request.ServiceCategoryID == "" {
				return errors.New(
					"ride request service category is required for dispatch",
				)
			}

			branchID := selectedAssignment.BranchID

			resolvedPricing, err := pricingService.Resolve(
				ctx,
				pricing.ResolveInput{
					CompanyID: selectedAssignment.CompanyID,
					BranchID:  &branchID,

					ServiceCategoryID: *request.ServiceCategoryID,

					At: now,
				},
			)
			if err != nil {
				return fmt.Errorf(
					"resolve dispatch pricing: %w",
					err,
				)
			}

			if resolvedPricing == nil ||
				resolvedPricing.ProfileID == "" {
				return errors.New(
					"dispatch pricing resolution returned no pricing profile",
				)
			}

			// ---------------------------------------------------------
			// 4. Build assigned trip
			// ---------------------------------------------------------

			pickupAddress := request.PickupAddress
			pickupLatitude := request.PickupLatitude
			pickupLongitude := request.PickupLongitude

			dropoffAddress := request.DestinationAddress
			dropoffLatitude := request.DestinationLatitude
			dropoffLongitude := request.DestinationLongitude

			passengerNote := request.Notes

			trip := &models.Trip{
				BaseModel: models.BaseModel{
					ID:        uuid.NewString(),
					CreatedAt: now,
					UpdatedAt: now,
				},

				RideRequestID: request.ID,
				CustomerID:    request.CustomerID,

				DriverID:  selectedAssignment.DriverID,
				VehicleID: selectedAssignment.VehicleID,
				FleetID:   selectedAssignment.FleetID,

				CompanyID:         selectedAssignment.CompanyID,
				BranchID:          selectedAssignment.BranchID,
				ServiceCategoryID: request.ServiceCategoryID,
				PricingProfileID:  &resolvedPricing.ProfileID,

				Status:     tripStatusAssigned,
				AssignedAt: now,

				PickupAddress:   &pickupAddress,
				PickupLatitude:  &pickupLatitude,
				PickupLongitude: &pickupLongitude,

				DropoffAddress:   &dropoffAddress,
				DropoffLatitude:  &dropoffLatitude,
				DropoffLongitude: &dropoffLongitude,

				PassengerNote: &passengerNote,

				IsActive: true,
			}

			// ---------------------------------------------------------
			// 5. Create trip
			// ---------------------------------------------------------

			if err := trips.Create(
				ctx,
				trip,
			); err != nil {
				return fmt.Errorf(
					"create dispatched trip: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 6. Mark selected driver BUSY
			// ---------------------------------------------------------

			if err := presence.UpdateAvailability(
				ctx,
				selected.DriverID,
				driverStatusBusy,
				true,
			); err != nil {
				return fmt.Errorf(
					"mark selected driver busy: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 7. Mark ride request ACCEPTED
			// ---------------------------------------------------------

			if err := rideRequests.UpdateStatus(
				ctx,
				request.ID,
				rideRequestStatusAccepted,
			); err != nil {
				return fmt.Errorf(
					"update ride request status: %w",
					err,
				)
			}

			dispatchedTrip = trip

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	if dispatchedTrip == nil {
		return nil, errors.New(
			"dispatch completed without creating a trip",
		)
	}

	return dispatchedTrip, nil
}
