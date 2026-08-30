package dispatch

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
	"github.com/JCKFinland/connect/backend/internal/services/pricing"
)

var (
	ErrDispatchOfferAlreadyResolved = errors.New(
		"dispatch offer is already resolved",
	)

	ErrDispatchOfferExpired = errors.New(
		"dispatch offer has expired",
	)

	ErrDispatchOfferDriverUnavailable = errors.New(
		"dispatch offer driver is no longer available",
	)
)

const (
	dispatchOfferStatusAccepted = "ACCEPTED"
	dispatchOfferStatusExpired  = "EXPIRED"
)

// AcceptOffer atomically accepts a PENDING dispatch offer.
//
// Successful acceptance performs all operational state changes in one
// PostgreSQL transaction:
//
//   - lock dispatch offer
//   - validate offer and ride-request expiration
//   - resolve drivers.id -> users.id
//   - lock driver presence and verify AVAILABLE
//   - lock ride request and verify MATCHING
//   - create ASSIGNED trip
//   - mark driver BUSY
//   - mark ride request ACCEPTED
//   - mark dispatch offer ACCEPTED
func (s *Service) AcceptOffer(
	ctx context.Context,
	offerID string,
) (*models.Trip, error) {

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

	if offerID == "" {
		return nil, errors.New(
			"dispatch offer ID is required",
		)
	}

	var (
		acceptedTrip       *models.Trip
		rideRequestExpired bool
	)

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			offers := postgresrepo.NewDispatchOfferRepositoryWithDB(tx)
			drivers := postgresrepo.NewDriverRepositoryWithDB(tx)
			presence := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)
			rideRequests := postgresrepo.NewRideRequestRepositoryWithDB(tx)
			trips := postgresrepo.NewTripRepositoryWithDB(tx)
			farePricingProfiles :=
				postgresrepo.NewFarePricingProfileRepositoryWithDB(tx)

			pricingService := pricing.NewService(
				pricing.Dependencies{
					FarePricingProfiles: farePricingProfiles,
				},
			)

			// ---------------------------------------------------------
			// 1. Lock dispatch offer.
			// ---------------------------------------------------------

			offer, err := offers.GetByIDForUpdate(
				ctx,
				offerID,
			)
			if err != nil {
				return fmt.Errorf(
					"get dispatch offer: %w",
					err,
				)
			}

			if offer.Status != dispatchOfferStatusPending {
				return ErrDispatchOfferAlreadyResolved
			}

			now := time.Now().UTC()

			// ---------------------------------------------------------
			// 2. Validate expiry.
			//
			// Do NOT update to EXPIRED here and then return an error,
			// because RunInTransaction would roll that update back.
			// Dedicated expiration processing handles that lifecycle.
			// ---------------------------------------------------------

			if !now.Before(offer.ExpiresAt) {
				return ErrDispatchOfferExpired
			}

			// ---------------------------------------------------------
			// 3. Resolve operational drivers.id -> users.id.
			// ---------------------------------------------------------

			driver, err := drivers.GetByID(
				ctx,
				offer.DriverID,
			)
			if err != nil {
				return fmt.Errorf(
					"resolve dispatch offer driver: %w",
					err,
				)
			}

			if driver == nil ||
				driver.UserID == "" ||
				!driver.IsActive {
				return fmt.Errorf(
					"dispatch offer driver is not active",
				)
			}

			// ---------------------------------------------------------
			// 4. Lock driver presence.
			//
			// Presence still uses users.id.
			// ---------------------------------------------------------

			lockedPresence, err :=
				presence.GetAvailableByDriverIDForUpdate(
					ctx,
					driver.UserID,
				)

			if errors.Is(err, repository.ErrNotFound) {
				return ErrDispatchOfferDriverUnavailable
			}

			if err != nil {
				return fmt.Errorf(
					"lock dispatch offer driver: %w",
					err,
				)
			}

			if lockedPresence == nil ||
				lockedPresence.AvailabilityStatus != "AVAILABLE" {
				return ErrDispatchOfferDriverUnavailable
			}

			// ---------------------------------------------------------
			// 5. Lock and validate ride request.
			// ---------------------------------------------------------

			request, err := rideRequests.GetByIDForUpdate(
				ctx,
				offer.RideRequestID,
			)
			if err != nil {
				return fmt.Errorf(
					"get ride request for offer acceptance: %w",
					err,
				)
			}

			if request.ExpiresAt != nil &&
				!now.Before(request.ExpiresAt.UTC()) {

				// ---------------------------------------------------------
				// The ride request itself has reached its hard expiry.
				//
				// Acceptance must terminate here:
				//   - no trip is created
				//   - driver remains AVAILABLE
				//   - ride becomes EXPIRED
				//   - retry state is cleared
				//   - pending offer becomes EXPIRED
				//
				// Do not return ErrRideRequestExpired from inside the
				// transaction after making these updates, because that would
				// roll them back. Commit first, then return the sentinel.
				// ---------------------------------------------------------

				if request.Status == rideRequestStatusMatching ||
					request.Status == rideRequestStatusPending {

					if err := rideRequests.UpdateStatus(
						ctx,
						request.ID,
						rideRequestStatusExpired,
					); err != nil {
						return fmt.Errorf(
							"expire ride request during offer acceptance: %w",
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

				if err := offers.UpdateStatus(
					ctx,
					offer.ID,
					dispatchOfferStatusExpired,
					&now,
					nil,
				); err != nil {
					return fmt.Errorf(
						"expire dispatch offer for expired ride request: %w",
						err,
					)
				}

				rideRequestExpired = true

				return nil
			}

			if request.Status != rideRequestStatusMatching {
				return fmt.Errorf(
					"ride request must be %s before offer acceptance",
					rideRequestStatusMatching,
				)
			}

			if request.ServiceCategoryID == nil ||
				*request.ServiceCategoryID == "" {
				return errors.New(
					"ride request service category is required for offer acceptance",
				)
			}

			branchID := offer.BranchID

			resolvedPricing, err := pricingService.Resolve(
				ctx,
				pricing.ResolveInput{
					CompanyID: offer.CompanyID,
					BranchID:  &branchID,

					ServiceCategoryID: *request.ServiceCategoryID,

					At: now,
				},
			)
			if err != nil {
				return fmt.Errorf(
					"resolve offer acceptance pricing: %w",
					err,
				)
			}

			if resolvedPricing == nil ||
				resolvedPricing.ProfileID == "" {
				return errors.New(
					"offer acceptance pricing resolution returned no pricing profile",
				)
			}

			// ---------------------------------------------------------
			// 6. Build ASSIGNED trip.
			//
			// trips.driver_id currently uses users.id, not drivers.id.
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

				DriverID: driver.UserID,

				VehicleID:         offer.VehicleID,
				CompanyID:         offer.CompanyID,
				BranchID:          offer.BranchID,
				ServiceCategoryID: request.ServiceCategoryID,
				PricingProfileID:  &resolvedPricing.ProfileID,
				FleetID:           offer.FleetID,

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
			// 7. Create trip.
			// ---------------------------------------------------------

			if err := trips.Create(
				ctx,
				trip,
			); err != nil {
				return fmt.Errorf(
					"create accepted trip: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 8. Driver becomes BUSY.
			//
			// Presence uses users.id.
			// ---------------------------------------------------------

			if err := presence.UpdateAvailability(
				ctx,
				driver.UserID,
				driverStatusBusy,
				true,
			); err != nil {
				return fmt.Errorf(
					"mark accepted driver busy: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 9. Ride request becomes ACCEPTED.
			// ---------------------------------------------------------

			if err := rideRequests.UpdateStatus(
				ctx,
				request.ID,
				rideRequestStatusAccepted,
			); err != nil {
				return fmt.Errorf(
					"accept ride request: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 10. Dispatch offer becomes ACCEPTED.
			// ---------------------------------------------------------

			if err := offers.UpdateStatus(
				ctx,
				offer.ID,
				dispatchOfferStatusAccepted,
				&now,
				nil,
			); err != nil {
				return fmt.Errorf(
					"accept dispatch offer: %w",
					err,
				)
			}

			acceptedTrip = trip

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	// The transaction has committed the terminal lifecycle state.
	// Return the domain error only after the EXPIRED changes are durable.
	if rideRequestExpired {
		return nil, ErrRideRequestExpired
	}

	if acceptedTrip == nil {
		return nil, errors.New(
			"dispatch offer acceptance completed without creating a trip",
		)
	}

	return acceptedTrip, nil
}

var ErrDispatchOfferAccessDenied = errors.New(
	"you are not authorized to accept this dispatch offer",
)

// AcceptOfferAuthorized verifies that the authenticated user owns the
// driver profile targeted by the offer before performing atomic acceptance.
func (s *Service) AcceptOfferAuthorized(
	ctx context.Context,
	offerID string,
	userID string,
) (*models.Trip, error) {

	if offerID == "" {
		return nil, errors.New(
			"dispatch offer ID is required",
		)
	}

	if userID == "" {
		return nil, errors.New(
			"authenticated user ID is required",
		)
	}

	offer, err := s.offers.GetByID(
		ctx,
		offerID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get dispatch offer for authorization: %w",
			err,
		)
	}

	driver, err := s.drivers.GetByUserID(
		ctx,
		userID,
	)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDispatchOfferAccessDenied
		}

		return nil, fmt.Errorf(
			"resolve authenticated driver: %w",
			err,
		)
	}

	if driver == nil ||
		driver.ID == "" ||
		driver.ID != offer.DriverID {

		return nil, ErrDispatchOfferAccessDenied
	}

	return s.AcceptOffer(
		ctx,
		offerID,
	)
}
