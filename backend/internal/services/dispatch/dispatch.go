package dispatch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
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

			// ---------------------------------------------------------
			// 1. Load and validate ride request
			// ---------------------------------------------------------

			request, err := rideRequests.GetByID(
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
			// 3. Find first fully eligible candidate
			//
			// A candidate must have:
			// - online + AVAILABLE presence
			// - a recent heartbeat
			// - an active driver/vehicle assignment
			// - an existing assigned vehicle
			// - a vehicle compatible with the requested ride type
			// ---------------------------------------------------------

			now := time.Now().UTC()

			var selected *models.DriverPresence
			var selectedAssignment *models.DriverAssignment
			nearestDistanceKM := math.MaxFloat64

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

				// Ignore stale heartbeats.
				if heartbeatAge > s.cfg.Presence.HeartbeatTimeout {
					continue
				}

				// Ignore obviously invalid future heartbeat timestamps.
				if heartbeatAge < 0 {
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

				if candidate.Latitude == nil || candidate.Longitude == nil {
					continue
				}

				candidateDistanceKM := distanceKM(
					*candidate.Latitude,
					*candidate.Longitude,
					request.PickupLatitude,
					request.PickupLongitude,
				)

				if candidateDistanceKM >= nearestDistanceKM {
					continue
				}

				nearestDistanceKM = candidateDistanceKM
				selected = candidate
				selectedAssignment = candidateAssignment

			}

			if selected == nil || selectedAssignment == nil {
				return ErrNoAvailableDrivers
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

				CompanyID: selectedAssignment.CompanyID,
				BranchID:  selectedAssignment.BranchID,

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
