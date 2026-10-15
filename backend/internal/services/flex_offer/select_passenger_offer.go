package flex_offer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	drivereligibility "github.com/JCKFinland/connect/backend/internal/services/driver_eligibility"
	vehicleeligibility "github.com/JCKFinland/connect/backend/internal/services/vehicle_eligibility"
	"github.com/jackc/pgx/v5"
)

var (
	ErrPassengerSelectionAccessDenied = errors.New(
		"FLEX passenger selection access denied",
	)

	ErrPassengerSelectionNotEligible = errors.New(
		"FLEX passenger selection is no longer eligible",
	)
)

// SelectPassengerOffer atomically selects a pending driver offer
// for an eligible FLEX ride request owned by the passenger.
//
// The operation reserves the offer and transitions the auction
// to PENDING_AUTHORIZATION.
//
// It does not authorize payment, create a trip, or assign a driver.
func (s *Service) SelectPassengerOffer(
	ctx context.Context,
	rideRequestID string,
	auctionID string,
	offerID string,
	passengerUserID string,
) (*models.FlexOffer, error) {
	if s == nil || s.db == nil || s.cfg == nil {
		return nil, errors.New("FLEX selection service is not configured")
	}

	if rideRequestID == "" ||
		auctionID == "" ||
		offerID == "" ||
		passengerUserID == "" {
		return nil, ErrPassengerSelectionAccessDenied
	}

	// Resolve ownership before exposing auction or offer information.
	request, err := postgresrepo.NewRideRequestRepository(s.db).
		GetByID(ctx, rideRequestID)
	if err != nil {
		return nil, err
	}
	if request == nil || request.CustomerID != passengerUserID {
		return nil, ErrPassengerSelectionAccessDenied
	}

	var selected *models.FlexOffer

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"ride-dispatch:"+rideRequestID,
			); err != nil {
				return fmt.Errorf("lock FLEX ride dispatch: %w", err)
			}

			request, err := postgresrepo.
				NewRideRequestRepositoryWithDB(tx).
				GetByIDForUpdate(ctx, rideRequestID)
			if err != nil {
				return fmt.Errorf("lock FLEX ride request: %w", err)
			}

			if request == nil ||
				request.CustomerID != passengerUserID {
				return ErrPassengerSelectionAccessDenied
			}

			auction, err := postgresrepo.
				NewFlexAuctionRepositoryWithDB(tx).
				GetByIDForUpdate(ctx, auctionID)
			if err != nil {
				return fmt.Errorf("lock FLEX auction: %w", err)
			}

			now := time.Now().UTC()

			if auction == nil ||
				auction.RideRequestID != request.ID ||
				request.MatchingMode != models.RideMatchingModeFlex ||
				request.Status != "PENDING" ||
				auction.Status != models.FlexAuctionStatusOpen ||
				auction.SelectedOfferID != nil ||
				auction.Currency != "EUR" ||
				auction.PassengerOfferCents <= 0 ||
				!now.Before(auction.ExpiresAt.UTC()) {
				return ErrPassengerSelectionNotEligible
			}

			if request.ExpiresAt != nil &&
				!now.Before(request.ExpiresAt.UTC()) {
				return ErrPassengerSelectionNotEligible
			}

			offer, err := postgresrepo.
				NewFlexOfferRepositoryWithDB(tx).
				GetByIDForUpdate(ctx, offerID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrPassengerSelectionNotEligible
				}
				return fmt.Errorf("lock FLEX offer: %w", err)
			}

			if offer == nil ||
				offer.AuctionID != auction.ID ||
				offer.Status != models.FlexOfferStatusPending ||
				!now.Before(offer.ExpiresAt.UTC()) ||
				offer.OfferedAmountCents <= 0 ||
				offer.OfferedAmountCents < auction.MinimumOfferCents ||
				offer.OfferedAmountCents > auction.MaximumOfferCents {
				return ErrPassengerSelectionNotEligible
			}

			switch offer.OfferType {
			case models.FlexOfferTypeAcceptFare:
				if offer.OfferedAmountCents != auction.PassengerOfferCents {
					return ErrPassengerSelectionNotEligible
				}
			case models.FlexOfferTypeCounterOffer:
				if offer.OfferedAmountCents == auction.PassengerOfferCents {
					return ErrPassengerSelectionNotEligible
				}
			default:
				return ErrPassengerSelectionNotEligible
			}

			expected := &drivereligibility.LifecycleExpectation{
				AssignmentID: offer.AssignmentID,
				CompanyID:    offer.CompanyID,
				BranchID:     offer.BranchID,
				FleetID:      offer.FleetID,
				VehicleID:    offer.VehicleID,
			}

			assignments := postgresrepo.
				NewDriverAssignmentRepositoryWithDB(tx)

			initialAssignment, err := assignments.GetActiveByDriver(
				ctx,
				offer.DriverID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrPassengerSelectionNotEligible
			}
			if err != nil {
				return fmt.Errorf("resolve FLEX driver assignment: %w", err)
			}

			if !drivereligibility.ValidAssignment(
				initialAssignment,
				offer.DriverID,
				expected,
			) || initialAssignment.UnassignedAt != nil {
				return ErrPassengerSelectionNotEligible
			}

			eligible, err := drivereligibility.LockAndValidateLifecycle(
				ctx,
				tx,
				offer.DriverID,
				expected,
				now,
			)
			if err != nil {
				return fmt.Errorf("validate FLEX driver lifecycle: %w", err)
			}
			if !eligible {
				return ErrPassengerSelectionNotEligible
			}

			presence, err := postgresrepo.
				NewDriverPresenceRepositoryWithDB(tx).
				GetAvailableByDriverIDForUpdate(
					ctx,
					offer.DriverID,
				)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrPassengerSelectionNotEligible
			}
			if err != nil {
				return fmt.Errorf("lock FLEX driver presence: %w", err)
			}

			if presence == nil ||
				presence.LastHeartbeatAt == nil ||
				s.cfg.Presence.HeartbeatTimeout <= 0 {
				return ErrPassengerSelectionNotEligible
			}

			presenceCheckedAt := time.Now().UTC()
			heartbeatAge := presenceCheckedAt.Sub(
				presence.LastHeartbeatAt.UTC(),
			)

			if heartbeatAge < 0 ||
				heartbeatAge > s.cfg.Presence.HeartbeatTimeout {
				return ErrPassengerSelectionNotEligible
			}

			assignment, err := assignments.GetActiveByDriver(
				ctx,
				offer.DriverID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrPassengerSelectionNotEligible
			}
			if err != nil {
				return fmt.Errorf("recheck FLEX driver assignment: %w", err)
			}

			if !drivereligibility.ValidAssignment(
				assignment,
				offer.DriverID,
				expected,
			) ||
				assignment.UnassignedAt != nil ||
				!drivereligibility.SameAssignment(
					initialAssignment,
					assignment,
				) {
				return ErrPassengerSelectionNotEligible
			}

			if presence.DriverID != offer.DriverID ||
				presence.AssignmentID == nil ||
				*presence.AssignmentID != offer.AssignmentID ||
				presence.VehicleID == nil ||
				*presence.VehicleID != offer.VehicleID ||
				presence.CompanyID != offer.CompanyID ||
				presence.BranchID == nil ||
				*presence.BranchID != offer.BranchID {
				return ErrPassengerSelectionNotEligible
			}

			vehicle, err := postgresrepo.
				NewVehicleRepositoryWithDB(tx).
				GetByID(ctx, offer.VehicleID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrPassengerSelectionNotEligible
			}
			if err != nil {
				return fmt.Errorf("resolve FLEX vehicle: %w", err)
			}

			if vehicle == nil ||
				!vehicle.IsActive ||
				!vehicleeligibility.IsEligible(
					request.RequestedVehicleType,
					vehicle.VehicleType,
				) {
				return ErrPassengerSelectionNotEligible
			}

			// Recheck deadlines after potentially blocking locks.
			var stillEligible bool
			err = tx.QueryRow(ctx, `
				SELECT
					a.status = 'OPEN'
					AND a.selected_offer_id IS NULL
					AND a.expires_at > clock_timestamp()
					AND o.status = 'PENDING'
					AND o.expires_at > clock_timestamp()
					AND r.status = 'PENDING'
					AND (
						r.expires_at IS NULL
						OR r.expires_at > clock_timestamp()
					)
				FROM flex_auctions a
				JOIN flex_offers o
					ON o.auction_id = a.id
				JOIN ride_requests r
					ON r.id = a.ride_request_id
				WHERE a.id = $1
				  AND o.id = $2
				  AND r.id = $3
			`, auctionID, offerID, rideRequestID).Scan(&stillEligible)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrPassengerSelectionNotEligible
			}
			if err != nil {
				return fmt.Errorf("recheck FLEX selection deadlines: %w", err)
			}
			if !stillEligible {
				return ErrPassengerSelectionNotEligible
			}

			if err := postgresrepo.SelectPendingOffer(
				ctx,
				tx,
				auctionID,
				offerID,
			); err != nil {
				if errors.Is(err, postgresrepo.ErrFlexSelectionConflict) {
					return ErrPassengerSelectionNotEligible
				}
				return err
			}

			if err := postgresrepo.ReserveSelectedOffer(
				ctx,
				tx,
				auctionID,
				offerID,
			); err != nil {
				if errors.Is(err, postgresrepo.ErrFlexSelectionConflict) {
					return ErrPassengerSelectionNotEligible
				}
				return err
			}

			offer.Status = models.FlexOfferStatusSelected
			selected = offer
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return selected, nil
}
