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
	rideRequestStatusExpired  = "EXPIRED"

	dispatchOfferStatusPending = "PENDING"

	defaultDispatchOfferTimeout = 30 * time.Second
)

var ErrRideRequestExpired = errors.New(
	"ride request has expired",
)

// CreateOffer selects the nearest eligible available driver and creates
// a PENDING dispatch offer without creating a trip or marking the driver BUSY.
//
// A driver who has already received an offer for the same ride request is
// excluded from subsequent dispatch attempts.
//
// Dispatch creation is protected by a PostgreSQL transaction-scoped advisory
// lock keyed by ride request ID. This prevents multiple CONNECT backend
// instances, workers, or API requests from dispatching the same ride
// concurrently.
func (s *Service) CreateOffer(
	ctx context.Context,
	rideRequestID string,
	createdByUserID string,
) (*models.DispatchOffer, error) {

	if s == nil {
		return nil, errors.New(
			"dispatch service is required",
		)
	}

	if s.db == nil {
		return nil, errors.New(
			"dispatch database is not configured",
		)
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

	var (
		createdOffer       *models.DispatchOffer
		rideRequestExpired bool
	)

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			// ---------------------------------------------------------
			// 1. Acquire cross-instance dispatch lock for this ride.
			//
			// This prevents multiple CONNECT backend instances,
			// workers, or administrator requests from dispatching
			// the same ride concurrently.
			//
			// PostgreSQL automatically releases this advisory lock
			// when the transaction commits or rolls back.
			// ---------------------------------------------------------

			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"ride-dispatch:"+rideRequestID,
			); err != nil {
				return fmt.Errorf(
					"lock ride request dispatch: %w",
					err,
				)
			}

			rideRequests :=
				postgresrepo.NewRideRequestRepositoryWithDB(tx)

			assignments :=
				postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)

			presence :=
				postgresrepo.NewDriverPresenceRepositoryWithDB(tx)

			vehicles :=
				postgresrepo.NewVehicleRepositoryWithDB(tx)

			drivers :=
				postgresrepo.NewDriverRepositoryWithDB(tx)

			offers :=
				postgresrepo.NewDispatchOfferRepositoryWithDB(tx)

			now := time.Now().UTC()

			// ---------------------------------------------------------
			// 2. Expire stale PENDING dispatch offers.
			// ---------------------------------------------------------

			expiredRideRequestIDs, err :=
				offers.ExpireStalePending(
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
			// 3. Recover ride requests belonging to stale offers.
			//
			// A still-valid MATCHING request returns to PENDING.
			// A ride whose own expires_at has been reached becomes
			// terminal EXPIRED instead.
			//
			// ACCEPTED, CANCELLED, EXPIRED, and other lifecycle
			// states are deliberately left untouched.
			// ---------------------------------------------------------

			for _, expiredRideRequestID := range expiredRideRequestIDs {

				if expiredRideRequestID == "" {
					continue
				}

				expiredRequest, err :=
					rideRequests.GetByIDForUpdate(
						ctx,
						expiredRideRequestID,
					)
				if err != nil {
					return fmt.Errorf(
						"get ride request after stale offer expiry: %w",
						err,
					)
				}

				if expiredRequest.Status !=
					rideRequestStatusMatching {

					continue
				}

				if expiredRequest.ExpiresAt != nil &&
					!now.Before(
						expiredRequest.ExpiresAt.UTC(),
					) {

					if err := rideRequests.UpdateStatus(
						ctx,
						expiredRequest.ID,
						rideRequestStatusExpired,
					); err != nil {
						return fmt.Errorf(
							"expire ride request after stale offer expiry: %w",
							err,
						)
					}

					if err := rideRequests.ResetDispatchRetry(
						ctx,
						expiredRequest.ID,
					); err != nil {
						return fmt.Errorf(
							"reset expired ride dispatch retry state: %w",
							err,
						)
					}

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
			// 4. Load drivers who already received this ride.
			//
			// dispatch_offers stores drivers.id.
			// driver_presence stores users.id.
			// ---------------------------------------------------------

			previouslyOfferedDriverIDs, err :=
				offers.ListDriverIDsByRideRequest(
					ctx,
					rideRequestID,
				)
			if err != nil {
				return fmt.Errorf(
					"list previously offered drivers: %w",
					err,
				)
			}

			previouslyOfferedDrivers := make(
				map[string]struct{},
				len(previouslyOfferedDriverIDs),
			)

			for _, driverID := range previouslyOfferedDriverIDs {

				if driverID == "" {
					continue
				}

				previouslyOfferedDrivers[driverID] =
					struct{}{}
			}

			// ---------------------------------------------------------
			// 5. Lock and validate requested ride request.
			//
			// The advisory lock protects the logical dispatch
			// operation, while FOR UPDATE protects the actual
			// ride_requests row.
			// ---------------------------------------------------------

			request, err :=
				rideRequests.GetByIDForUpdate(
					ctx,
					rideRequestID,
				)
			if err != nil {
				return fmt.Errorf(
					"get ride request: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// Enforce ride-request hard expiration.
			//
			// expires_at is the authoritative end of the matching
			// lifecycle. Once reached, the ride may no longer receive
			// another dispatch offer.
			//
			// NULL expires_at means no configured hard expiration.
			//
			// Important:
			// Do not return ErrRideRequestExpired from inside this
			// transaction after changing state. Doing so would roll
			// back the EXPIRED update. Instead, commit the state first
			// and return the sentinel after RunInTransaction finishes.
			// ---------------------------------------------------------

			if request.ExpiresAt != nil &&
				!now.Before(request.ExpiresAt.UTC()) {

				if request.Status ==
					rideRequestStatusPending ||
					request.Status ==
						rideRequestStatusMatching {

					if err := rideRequests.UpdateStatus(
						ctx,
						request.ID,
						rideRequestStatusExpired,
					); err != nil {
						return fmt.Errorf(
							"expire ride request before dispatch: %w",
							err,
						)
					}

					if err := rideRequests.ResetDispatchRetry(
						ctx,
						request.ID,
					); err != nil {
						return fmt.Errorf(
							"reset expired ride dispatch retry state: %w",
							err,
						)
					}
				}

				rideRequestExpired = true

				return nil
			}

			if request.Status != rideRequestStatusPending {
				return fmt.Errorf(
					"ride request must be %s before dispatch",
					rideRequestStatusPending,
				)
			}

			// ---------------------------------------------------------
			// 6. Discover AVAILABLE drivers.
			// ---------------------------------------------------------

			availableDrivers, err :=
				presence.ListAllAvailable(ctx)
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
			// 7. Filter and rank eligible drivers.
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
					heartbeatAge >
						s.cfg.Presence.HeartbeatTimeout {

					continue
				}

				// Resolve users.id -> drivers.id.
				operationalDriver, err :=
					drivers.GetByUserID(
						ctx,
						candidate.DriverID,
					)

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"resolve candidate operational driver: %w",
						err,
					)
				}

				if operationalDriver == nil ||
					operationalDriver.ID == "" ||
					!operationalDriver.IsActive {

					continue
				}

				// Never offer the same ride to the same driver again.
				_, alreadyOffered := previouslyOfferedDrivers[operationalDriver.ID]

				if alreadyOffered {
					continue
				}

				assignment, err :=
					assignments.GetActiveByDriver(
						ctx,
						candidate.DriverID,
					)

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
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

				vehicle, err :=
					vehicles.GetByID(
						ctx,
						assignment.VehicleID,
					)

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"get candidate vehicle: %w",
						err,
					)
				}

				if vehicle == nil ||
					!vehicle.IsActive {

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
			// 8. Lock nearest still-available eligible candidate.
			// ---------------------------------------------------------

			var selected *models.DriverPresence

			var selectedAssignment *models.DriverAssignment

			for _, candidate := range candidates {

				lockedDriver, err :=
					presence.GetAvailableByDriverIDForUpdate(
						ctx,
						candidate.presence.DriverID,
					)

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"lock candidate driver: %w",
						err,
					)
				}

				if lockedDriver == nil ||
					lockedDriver.LastHeartbeatAt == nil {

					continue
				}

				lockedHeartbeatAge := now.Sub(
					lockedDriver.LastHeartbeatAt.UTC(),
				)

				if lockedHeartbeatAge < 0 ||
					lockedHeartbeatAge >
						s.cfg.Presence.HeartbeatTimeout {

					continue
				}

				// Re-resolve the operational driver after acquiring
				// the presence lock.
				lockedOperationalDriver, err :=
					drivers.GetByUserID(
						ctx,
						lockedDriver.DriverID,
					)

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
					continue
				}

				if err != nil {
					return fmt.Errorf(
						"recheck operational driver: %w",
						err,
					)
				}

				if lockedOperationalDriver == nil ||
					lockedOperationalDriver.ID == "" ||
					!lockedOperationalDriver.IsActive {

					continue
				}

				_, alreadyOffered := previouslyOfferedDrivers[lockedOperationalDriver.ID]

				if alreadyOffered {
					continue
				}

				lockedAssignment, err :=
					assignments.GetActiveByDriver(
						ctx,
						lockedDriver.DriverID,
					)

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
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

				lockedVehicle, err :=
					vehicles.GetByID(
						ctx,
						lockedAssignment.VehicleID,
					)

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
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
			// 9. Resolve final users.id -> drivers.id.
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

			// Final defensive previously-offered check.
			_, wasAlreadyOffered :=
				previouslyOfferedDrivers[driver.ID]

			if wasAlreadyOffered {
				return ErrNoAvailableDrivers
			}

			// ---------------------------------------------------------
			// 10. Calculate offer expiration.
			//
			// The dispatch offer may never remain valid beyond the
			// ride request's own hard expiration.
			// ---------------------------------------------------------

			offerExpiresAt := now.Add(
				defaultDispatchOfferTimeout,
			)

			if request.ExpiresAt != nil &&
				request.ExpiresAt.UTC().Before(
					offerExpiresAt,
				) {

				offerExpiresAt =
					request.ExpiresAt.UTC()
			}

			// ---------------------------------------------------------
			// 11. Create PENDING dispatch offer.
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
				ExpiresAt: offerExpiresAt,

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
			// 12. Reset automatic redispatch backoff.
			//
			// Once an offer is successfully created, any previous
			// no-driver retry state is no longer relevant.
			// ---------------------------------------------------------

			if err := rideRequests.ResetDispatchRetry(
				ctx,
				request.ID,
			); err != nil {
				return fmt.Errorf(
					"reset ride dispatch retry state: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 13. Ride request becomes MATCHING.
			//
			// No trip exists yet and the selected driver remains
			// AVAILABLE until the offer is accepted.
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

	// The transaction has committed the terminal EXPIRED state,
	// so the caller can now safely receive the lifecycle error.
	if rideRequestExpired {
		return nil, ErrRideRequestExpired
	}

	if createdOffer == nil {
		return nil, errors.New(
			"dispatch completed without creating an offer",
		)
	}

	return createdOffer, nil
}
