package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"

	"github.com/jackc/pgx/v5"
)

const (
	dispatchOfferStatusRejected = "REJECTED"
)

// RejectOffer atomically rejects a PENDING dispatch offer.
//
// Successful rejection performs these changes in one PostgreSQL transaction:
//
//   - lock dispatch offer
//   - validate PENDING
//   - if already expired, mark EXPIRED
//   - otherwise mark REJECTED
//   - reset associated ride request MATCHING -> PENDING
//
// The ride request can then be offered to another eligible driver.
func (s *Service) RejectOffer(
	ctx context.Context,
	offerID string,
	reason string,
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

	if offerID == "" {
		return nil, errors.New(
			"dispatch offer ID is required",
		)
	}

	var (
		resolvedOffer *models.DispatchOffer
		wasExpired    bool
	)

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			offers := postgresrepo.NewDispatchOfferRepositoryWithDB(
				tx,
			)

			rideRequests := postgresrepo.NewRideRequestRepositoryWithDB(
				tx,
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
			// 2. Determine whether the offer has already expired.
			// ---------------------------------------------------------

			if !now.Before(offer.ExpiresAt) {

				if err := offers.UpdateStatus(
					ctx,
					offer.ID,
					dispatchOfferStatusExpired,
					&now,
					nil,
				); err != nil {
					return fmt.Errorf(
						"expire dispatch offer: %w",
						err,
					)
				}

				offer.Status = dispatchOfferStatusExpired
				offer.RespondedAt = &now
				offer.RejectionReason = nil
				offer.UpdatedAt = now

				wasExpired = true

			} else {

				var rejectionReason *string

				if reason != "" {
					rejectionReason = &reason
				}

				if err := offers.UpdateStatus(
					ctx,
					offer.ID,
					dispatchOfferStatusRejected,
					&now,
					rejectionReason,
				); err != nil {
					return fmt.Errorf(
						"reject dispatch offer: %w",
						err,
					)
				}

				offer.Status = dispatchOfferStatusRejected
				offer.RespondedAt = &now
				offer.RejectionReason = rejectionReason
				offer.UpdatedAt = now
			}

			// ---------------------------------------------------------
			// 3. Lock associated ride request.
			// ---------------------------------------------------------

			request, err := rideRequests.GetByIDForUpdate(
				ctx,
				offer.RideRequestID,
			)
			if err != nil {
				return fmt.Errorf(
					"get ride request after dispatch offer resolution: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 4. Restore MATCHING request to PENDING.
			//
			// Never move ACCEPTED/CANCELLED/EXPIRED requests backward.
			// ---------------------------------------------------------

			if request.Status == rideRequestStatusMatching {

				if err := rideRequests.UpdateStatus(
					ctx,
					request.ID,
					rideRequestStatusPending,
				); err != nil {
					return fmt.Errorf(
						"reset ride request after dispatch offer resolution: %w",
						err,
					)
				}
			}

			resolvedOffer = offer

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	if resolvedOffer == nil {
		return nil, errors.New(
			"dispatch offer rejection completed without resolving an offer",
		)
	}

	if wasExpired {
		return resolvedOffer, ErrDispatchOfferExpired
	}

	return resolvedOffer, nil
}

// RejectOfferAuthorized verifies that the authenticated user owns the
// driver profile targeted by the offer before allowing rejection.
func (s *Service) RejectOfferAuthorized(
	ctx context.Context,
	offerID string,
	userID string,
	reason string,
) (*models.DispatchOffer, error) {

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

	// ---------------------------------------------------------
	// 1. Load offer for ownership check.
	// ---------------------------------------------------------

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

	// ---------------------------------------------------------
	// 2. Resolve authenticated users.id -> drivers.id.
	// ---------------------------------------------------------

	driver, err := s.drivers.GetByUserID(
		ctx,
		userID,
	)
	if err != nil {

		if errors.Is(
			err,
			repository.ErrNotFound,
		) {
			return nil, ErrDispatchOfferAccessDenied
		}

		return nil, fmt.Errorf(
			"resolve authenticated driver: %w",
			err,
		)
	}

	// ---------------------------------------------------------
	// 3. Enforce offer ownership.
	// ---------------------------------------------------------

	if driver == nil ||
		driver.ID == "" ||
		driver.ID != offer.DriverID {

		return nil, ErrDispatchOfferAccessDenied
	}

	return s.RejectOffer(
		ctx,
		offerID,
		reason,
	)
}
