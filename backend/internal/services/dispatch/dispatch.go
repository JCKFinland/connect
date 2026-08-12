package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrNoAvailableDrivers = errors.New(
	"no available drivers found",
)

func (s *Service) DispatchRide(
	ctx context.Context,
	rideRequestID string,
) (*models.Trip, error) {

	if rideRequestID == "" {
		return nil, fmt.Errorf("ride request ID is required")
	}

	var dispatchedTrip *models.Trip

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			rideRequests := postgresrepo.NewRideRequestRepositoryWithDB(tx)
			assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)
			presence := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)
			trips := postgresrepo.NewTripRepositoryWithDB(tx)

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

			if request.Status != "PENDING" {
				return fmt.Errorf(
					"ride request must be PENDING before dispatch",
				)
			}

			availableDrivers, err := presence.ListAllAvailable(
				ctx,
			)
			if err != nil {
				return fmt.Errorf(
					"list available drivers: %w",
					err,
				)
			}

			if len(availableDrivers) == 0 {
				return ErrNoAvailableDrivers
			}

			now := time.Now().UTC()

			var selected *models.DriverPresence

			for _, candidate := range availableDrivers {

				if candidate.LastHeartbeatAt == nil {
					continue
				}

				if now.Sub(
					*candidate.LastHeartbeatAt,
				) > s.cfg.Presence.HeartbeatTimeout {
					continue
				}

				selected = candidate
				break
			}

			if selected == nil {
				return ErrNoAvailableDrivers
			}

			assignment, err := assignments.GetActiveByDriver(
				ctx,
				selected.DriverID,
			)
			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return fmt.Errorf(
					"active assignment not found for selected driver",
				)
			}

			if err != nil {
				return fmt.Errorf(
					"get selected driver assignment: %w",
					err,
				)
			}

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

				DriverID:  assignment.DriverID,
				VehicleID: assignment.VehicleID,
				FleetID:   assignment.FleetID,

				CompanyID: assignment.CompanyID,
				BranchID:  assignment.BranchID,

				Status:     "ASSIGNED",
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

			if err := trips.Create(
				ctx,
				trip,
			); err != nil {
				return fmt.Errorf(
					"create dispatched trip: %w",
					err,
				)
			}

			if err := presence.UpdateAvailability(
				ctx,
				selected.DriverID,
				"BUSY",
				true,
			); err != nil {
				return fmt.Errorf(
					"mark selected driver busy: %w",
					err,
				)
			}

			if err := rideRequests.UpdateStatus(
				ctx,
				request.ID,
				"ACCEPTED",
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

	return dispatchedTrip, nil
}
