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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	rideRequestStatusMatching = "MATCHING"

	dispatchOfferStatusPending = "PENDING"

	defaultDispatchOfferTimeout = 30 * time.Second
)

// CreateOffer selects the nearest eligible available driver and creates
// a PENDING dispatch offer without creating a trip or marking the driver BUSY.
func (s *Service) CreateOffer(
	ctx context.Context,
	rideRequestID string,
	createdByUserID string,
) (*models.DispatchOffer, error) {

	if s == nil {
		return nil, errors.New("dispatch service is required")
	}

	if s.db == nil {
		return nil, errors.New("dispatch database is not configured")
	}

	if s.cfg == nil {
		return nil, errors.New(
			"dispatch configuration is not configured",
		)
	}

	if s.cfg.Presence.HeartbeatTimeout <= 0 {
		return nil, errors.New(
			"presence heartbeat timeout must be greater than zero",
		)
	}

	if rideRequestID == "" {
		return nil, errors.New(
			"ride request ID is required",
		)
	}

	var createdOffer *models.DispatchOffer

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			rideRequests := postgresrepo.NewRideRequestRepositoryWithDB(tx)
			assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)
			presence := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)
			vehicles := postgresrepo.NewVehicleRepositoryWithDB(tx)
			drivers := postgresrepo.NewDriverRepositoryWithDB(tx)
			offers := postgresrepo.NewDispatchOfferRepositoryWithDB(tx)

			// ---------------------------------------------------------
			// 1. Expire stale PENDING dispatch offers.
			//
			// This releases the unique PENDING constraints before
			// selecting a driver for a new offer.
			// ---------------------------------------------------------

			now := time.Now().UTC()
			expiredRideRequestIDs, err := offers.ExpireStalePending(
				ctx,
				now,
			)
			if err != nil {
				return fmt.Errorf(
					"expire stale dispatch offers: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 2. Recover ride requests belonging to stale offers.
			//
			// Only MATCHING requests are moved back to PENDING.
			// ACCEPTED, CANCELLED, EXPIRED, or any other states are
			// deliberately left untouched.
			// ---------------------------------------------------------

			for _, expiredRideRequestID := range expiredRideRequestIDs {

				if expiredRideRequestID == "" {
					continue
				}

				expiredRequest, err := rideRequests.GetByIDForUpdate(
					ctx,
					expiredRideRequestID,
				)
				if err != nil {
					return fmt.Errorf(
						"get ride request after stale offer expiry: %w",
						err,
					)
				}

				if expiredRequest.Status != rideRequestStatusMatching {
					continue
				}

				if err := rideRequests.UpdateStatus(
					ctx,
					expiredRequest.ID,
					rideRequestStatusPending,
				); err != nil {
					return fmt.Errorf(
						"reset ride request after stale offer expiry: %w",
						err,
					)
				}
			}

			// ---------------------------------------------------------
			// 3. Lock and validate requested ride request.
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
			// 4. Discover AVAILABLE drivers.
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

			candidates := make(
				[]rankedCandidate,
				0,
				len(availableDrivers),
			)

			// ---------------------------------------------------------
			// 5. Filter and rank eligible drivers.
			// ---------------------------------------------------------

			for _, candidate := range availableDrivers {

				if candidate == nil ||
					candidate.DriverID == "" ||
					candidate.LastHeartbeatAt == nil ||
					candidate.Latitude == nil ||
					candidate.Longitude == nil {
					continue
				}

				heartbeatAge := now.Sub(
					candidate.LastHeartbeatAt.UTC(),
				)

				if heartbeatAge < 0 ||
					heartbeatAge > s.cfg.Presence.HeartbeatTimeout {
					continue
				}

				assignment, err := assignments.GetActiveByDriver(
					ctx,
					candidate.DriverID,
				)
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"get candidate assignment: %w",
						err,
					)
				}

				if assignment == nil ||
					assignment.VehicleID == "" {
					continue
				}

				vehicle, err := vehicles.GetByID(
					ctx,
					assignment.VehicleID,
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

				distance := distanceKM(
					*candidate.Latitude,
					*candidate.Longitude,
					request.PickupLatitude,
					request.PickupLongitude,
				)

				candidates = append(
					candidates,
					rankedCandidate{
						presence:   candidate,
						assignment: assignment,
						distanceKM: distance,
					},
				)
			}

			if len(candidates) == 0 {
				return ErrNoAvailableDrivers
			}

			sort.SliceStable(
				candidates,
				func(i, j int) bool {
					return candidates[i].distanceKM <
						candidates[j].distanceKM
				},
			)

			// ---------------------------------------------------------
			// 6. Lock nearest still-available candidate.
			// ---------------------------------------------------------

			var selected *models.DriverPresence
			var selectedAssignment *models.DriverAssignment

			for _, candidate := range candidates {

				lockedDriver, err := presence.GetAvailableByDriverIDForUpdate(
					ctx,
					candidate.presence.DriverID,
				)

				if errors.Is(err, repository.ErrNotFound) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"lock candidate driver: %w",
						err,
					)
				}

				if lockedDriver.LastHeartbeatAt == nil {
					continue
				}

				heartbeatAge := now.Sub(
					lockedDriver.LastHeartbeatAt.UTC(),
				)

				if heartbeatAge < 0 ||
					heartbeatAge > s.cfg.Presence.HeartbeatTimeout {
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
						"recheck driver assignment: %w",
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
						"recheck driver vehicle: %w",
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

			if selected == nil ||
				selectedAssignment == nil {
				return ErrNoAvailableDrivers
			}

			// ---------------------------------------------------------
			// 7. Convert users.id → drivers.id.
			// ---------------------------------------------------------

			driver, err := drivers.GetByUserID(
				ctx,
				selected.DriverID,
			)
			if err != nil {
				return fmt.Errorf(
					"resolve operational driver: %w",
					err,
				)
			}

			if driver == nil ||
				driver.ID == "" ||
				!driver.IsActive {
				return fmt.Errorf(
					"selected driver has no active driver record",
				)
			}

			// ---------------------------------------------------------
			// 8. Create PENDING offer.
			// ---------------------------------------------------------

			offer := &models.DispatchOffer{
				ID: uuid.NewString(),

				RideRequestID: request.ID,

				DriverID:  driver.ID,
				VehicleID: selectedAssignment.VehicleID,

				CompanyID: selectedAssignment.CompanyID,
				BranchID:  selectedAssignment.BranchID,
				FleetID:   selectedAssignment.FleetID,

				Status: dispatchOfferStatusPending,

				OfferedAt: now,
				ExpiresAt: now.Add(
					defaultDispatchOfferTimeout,
				),

				CreatedAt: now,
				UpdatedAt: now,
			}

			if createdByUserID != "" {
				offer.CreatedBy = &createdByUserID
			}

			if err := offers.Create(
				ctx,
				offer,
			); err != nil {
				return fmt.Errorf(
					"create dispatch offer: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 9. Ride request becomes MATCHING.
			//
			// Important:
			// - no trip yet
			// - driver remains AVAILABLE
			// - request is not ACCEPTED yet
			// ---------------------------------------------------------

			if err := rideRequests.UpdateStatus(
				ctx,
				request.ID,
				rideRequestStatusMatching,
			); err != nil {
				return fmt.Errorf(
					"mark ride request matching: %w",
					err,
				)
			}

			createdOffer = offer

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	if createdOffer == nil {
		return nil, errors.New(
			"dispatch completed without creating an offer",
		)
	}

	return createdOffer, nil
}
