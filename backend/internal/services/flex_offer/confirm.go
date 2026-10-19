package flex_offer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	drivereligibility "github.com/JCKFinland/connect/backend/internal/services/driver_eligibility"
	vehicleeligibility "github.com/JCKFinland/connect/backend/internal/services/vehicle_eligibility"
)

var ErrFlexConfirmationNotEligible = errors.New(
	"FLEX ride confirmation is not eligible",
)

var ErrFlexConfirmationAccessDenied = errors.New(
	"FLEX ride confirmation access denied",
)

// ConfirmAuthorizedRide assigns a passenger-selected FLEX offer only after
// its matching Stripe authorization has reached AUTHORIZED.
//
// Stripe operations must occur outside this database transaction.
func (s *Service) ConfirmAuthorizedRide(
	ctx context.Context,
	rideRequestID string,
	auctionID string,
	passengerUserID string,
) (*models.Trip, error) {
	if s == nil || s.db == nil || s.cfg == nil {
		return nil, errors.New("FLEX confirmation service is not configured")
	}

	for _, id := range []string{
		rideRequestID, auctionID, passengerUserID,
	} {
		if _, err := uuid.Parse(id); err != nil {
			return nil, ErrFlexConfirmationAccessDenied
		}
	}

	var confirmed *models.Trip

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx, tx, "ride-dispatch:"+rideRequestID,
			); err != nil {
				return fmt.Errorf("lock FLEX confirmation dispatch: %w", err)
			}

			rides := postgresrepo.NewRideRequestRepositoryWithDB(tx)
			request, err := rides.GetByIDForUpdate(ctx, rideRequestID)
			if err != nil {
				return fmt.Errorf("lock FLEX confirmation ride: %w", err)
			}
			if request == nil || request.CustomerID != passengerUserID {
				return ErrFlexConfirmationAccessDenied
			}

			auctions := postgresrepo.NewFlexAuctionRepositoryWithDB(tx)
			auction, err := auctions.GetByIDForUpdate(ctx, auctionID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrFlexConfirmationNotEligible
			}
			if err != nil {
				return fmt.Errorf("lock FLEX confirmation auction: %w", err)
			}

			if auction == nil ||
				auction.RideRequestID != request.ID ||
				request.MatchingMode != models.RideMatchingModeFlex ||
				request.Status != "PENDING" ||
				auction.Status != models.FlexAuctionStatusPendingAuthorization ||
				auction.SelectedOfferID == nil ||
				auction.Currency != "EUR" ||
				auction.PricingProfileID == "" ||
				auction.PricingVersion == "" {
				return ErrFlexConfirmationNotEligible
			}

			offers := postgresrepo.NewFlexOfferRepositoryWithDB(tx)
			offer, err := offers.GetByIDForUpdate(
				ctx, *auction.SelectedOfferID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrFlexConfirmationNotEligible
			}
			if err != nil {
				return fmt.Errorf("lock FLEX confirmation offer: %w", err)
			}
			if offer == nil ||
				offer.AuctionID != auction.ID ||
				offer.Status != models.FlexOfferStatusSelected ||
				offer.OfferedAmountCents <= 0 {
				return ErrFlexConfirmationNotEligible
			}

			var authorizationID string
			var authorizationAmount string
			var authorizationCurrency string
			var authorizationProvider string
			var authorizationStatus string
			var providerAuthorizationID *string

			err = tx.QueryRow(ctx, `
				SELECT
					id,
					amount::text,
					currency,
					provider,
					status,
					provider_authorization_id
				FROM flex_authorizations
				WHERE auction_id = $1
				  AND offer_id = $2
				  AND passenger_id = $3
				FOR UPDATE
			`, auction.ID, offer.ID, passengerUserID).Scan(
				&authorizationID,
				&authorizationAmount,
				&authorizationCurrency,
				&authorizationProvider,
				&authorizationStatus,
				&providerAuthorizationID,
			)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrFlexConfirmationNotEligible
			}
			if err != nil {
				return fmt.Errorf("lock FLEX confirmation authorization: %w", err)
			}

			// Compare exact decimal currency values without floating-point
			// conversion. PostgreSQL NUMERIC(10,2) renders two decimals.
			expectedAmount := fmt.Sprintf(
				"%d.%02d",
				offer.OfferedAmountCents/100,
				offer.OfferedAmountCents%100,
			)

			if authorizationID == "" ||
				authorizationStatus != "AUTHORIZED" ||
				authorizationProvider != "STRIPE" ||
				authorizationCurrency != "EUR" ||
				authorizationAmount != expectedAmount ||
				providerAuthorizationID == nil ||
				*providerAuthorizationID == "" {
				return ErrFlexConfirmationNotEligible
			}

			now := time.Now().UTC()

			expected := &drivereligibility.LifecycleExpectation{
				AssignmentID: offer.AssignmentID,
				CompanyID:    offer.CompanyID,
				BranchID:     offer.BranchID,
				FleetID:      offer.FleetID,
				VehicleID:    offer.VehicleID,
			}

			assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)
			initialAssignment, err := assignments.GetActiveByDriver(
				ctx, offer.DriverID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrFlexConfirmationNotEligible
			}
			if err != nil {
				return fmt.Errorf("resolve FLEX confirmation assignment: %w", err)
			}
			if !drivereligibility.ValidAssignment(
				initialAssignment, offer.DriverID, expected,
			) || initialAssignment.UnassignedAt != nil {
				return ErrFlexConfirmationNotEligible
			}

			eligible, err := drivereligibility.LockAndValidateLifecycle(
				ctx, tx, offer.DriverID, expected, now,
			)
			if err != nil {
				return fmt.Errorf("validate FLEX confirmation lifecycle: %w", err)
			}
			if !eligible {
				return ErrFlexConfirmationNotEligible
			}

			presenceRepo := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)
			presence, err := presenceRepo.GetAvailableByDriverIDForUpdate(
				ctx, offer.DriverID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrFlexConfirmationNotEligible
			}
			if err != nil {
				return fmt.Errorf("lock FLEX confirmation presence: %w", err)
			}
			if presence == nil ||
				presence.LastHeartbeatAt == nil ||
				s.cfg.Presence.HeartbeatTimeout <= 0 {
				return ErrFlexConfirmationNotEligible
			}

			presenceCheckedAt := time.Now().UTC()
			heartbeatAge := presenceCheckedAt.Sub(
				presence.LastHeartbeatAt.UTC(),
			)
			if heartbeatAge < 0 ||
				heartbeatAge > s.cfg.Presence.HeartbeatTimeout {
				return ErrFlexConfirmationNotEligible
			}

			currentAssignment, err := assignments.GetActiveByDriver(
				ctx, offer.DriverID,
			)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrFlexConfirmationNotEligible
				}
				return fmt.Errorf("recheck FLEX confirmation assignment: %w", err)
			}
			if !drivereligibility.ValidAssignment(
				currentAssignment, offer.DriverID, expected,
			) ||
				currentAssignment.UnassignedAt != nil ||
				!drivereligibility.SameAssignment(
					initialAssignment, currentAssignment,
				) {
				return ErrFlexConfirmationNotEligible
			}

			if presence.DriverID != offer.DriverID ||
				presence.AssignmentID == nil ||
				*presence.AssignmentID != offer.AssignmentID ||
				presence.VehicleID == nil ||
				*presence.VehicleID != offer.VehicleID ||
				presence.CompanyID != offer.CompanyID ||
				presence.BranchID == nil ||
				*presence.BranchID != offer.BranchID {
				return ErrFlexConfirmationNotEligible
			}

			vehicle, err := postgresrepo.
				NewVehicleRepositoryWithDB(tx).
				GetByID(ctx, offer.VehicleID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrFlexConfirmationNotEligible
				}
				return fmt.Errorf("resolve FLEX confirmation vehicle: %w", err)
			}
			if vehicle == nil ||
				!vehicle.IsActive ||
				!vehicleeligibility.IsEligible(
					request.RequestedVehicleType,
					vehicle.VehicleType,
				) {
				return ErrFlexConfirmationNotEligible
			}

			// Validate frozen auction pricing provenance against the
			// selected driver's organization before creating a trip.
			//
			// Do not require IsActive here: the auction may have
			// captured a valid historical pricing version.
			pricingProfiles := postgresrepo.
				NewFarePricingProfileRepositoryWithDB(tx)

			pricingProfile, err := pricingProfiles.GetByID(
				ctx, auction.PricingProfileID,
			)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrFlexConfirmationNotEligible
			}
			if err != nil {
				return fmt.Errorf(
					"resolve FLEX confirmation pricing profile: %w",
					err,
				)
			}

			if pricingProfile == nil ||
				pricingProfile.CompanyID != offer.CompanyID ||
				pricingProfile.Version != auction.PricingVersion ||
				pricingProfile.Currency != auction.Currency ||
				pricingProfile.Currency != "EUR" ||
				(pricingProfile.BranchID != nil &&
					*pricingProfile.BranchID != offer.BranchID) ||
				(request.ServiceCategoryID != nil &&
					*request.ServiceCategoryID !=
						pricingProfile.ServiceCategoryID) {
				return ErrFlexConfirmationNotEligible
			}

			pickupAddress := request.PickupAddress
			pickupLatitude := request.PickupLatitude
			pickupLongitude := request.PickupLongitude
			dropoffAddress := request.DestinationAddress
			dropoffLatitude := request.DestinationLatitude
			dropoffLongitude := request.DestinationLongitude
			passengerNote := request.Notes
			pricingProfileID := auction.PricingProfileID

			trip := &models.Trip{
				BaseModel: models.BaseModel{
					ID:        uuid.NewString(),
					CreatedAt: now,
					UpdatedAt: now,
				},
				RideRequestID:     request.ID,
				CustomerID:        request.CustomerID,
				DriverID:          offer.DriverID,
				VehicleID:         offer.VehicleID,
				CompanyID:         offer.CompanyID,
				BranchID:          offer.BranchID,
				FleetID:           offer.FleetID,
				ServiceCategoryID: request.ServiceCategoryID,
				PricingProfileID:  &pricingProfileID,
				Status:            "ASSIGNED",
				AssignedAt:        now,
				PickupAddress:     &pickupAddress,
				PickupLatitude:    &pickupLatitude,
				PickupLongitude:   &pickupLongitude,
				DropoffAddress:    &dropoffAddress,
				DropoffLatitude:   &dropoffLatitude,
				DropoffLongitude:  &dropoffLongitude,
				PassengerNote:     &passengerNote,
				IsActive:          true,
			}

			trips := postgresrepo.NewTripRepositoryWithDB(tx)
			if err := trips.Create(ctx, trip); err != nil {
				return fmt.Errorf("create confirmed FLEX trip: %w", err)
			}

			claimed, err := presenceRepo.ClaimForTrip(
				ctx, offer.DriverID, trip.ID,
			)
			if err != nil {
				return fmt.Errorf("claim confirmed FLEX driver: %w", err)
			}
			if !claimed {
				return ErrFlexConfirmationNotEligible
			}

			if err := rides.UpdateStatus(
				ctx, request.ID, "PENDING", "ACCEPTED",
			); err != nil {
				return fmt.Errorf("accept confirmed FLEX ride: %w", err)
			}

			if err := postgresrepo.AcceptSelectedFlexOffer(
				ctx, tx, auction.ID, offer.ID,
			); err != nil {
				return fmt.Errorf("accept confirmed FLEX offer: %w", err)
			}

			if err := postgresrepo.ConfirmAuthorizedFlexAuction(
				ctx, tx, auction.ID, offer.ID,
			); err != nil {
				return fmt.Errorf("finalize confirmed FLEX auction: %w", err)
			}

			confirmed = trip
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return confirmed, nil
}
