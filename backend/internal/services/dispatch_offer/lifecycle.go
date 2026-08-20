package dispatch_offer

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

var (
	ErrOfferAlreadyResolved = errors.New(
		"dispatch offer is already resolved",
	)

	ErrOfferExpired = errors.New(
		"dispatch offer has expired",
	)
)

// Accept transitions a PENDING dispatch offer to ACCEPTED.
//
// This service-level method changes only the dispatch-offer lifecycle.
// The production ride-acceptance workflow uses dispatch.AcceptOffer(),
// which atomically creates the trip, marks the driver BUSY,
// accepts the ride request, and accepts the dispatch offer.
func (s *Service) Accept(
	ctx context.Context,
	id string,
) (*models.DispatchOffer, error) {

	if id == "" {
		return nil, fmt.Errorf(
			"dispatch offer ID is required",
		)
	}

	if s.db == nil {
		return nil, fmt.Errorf(
			"dispatch offer database is not configured",
		)
	}

	var (
		accepted   *models.DispatchOffer
		wasExpired bool
	)

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			offers := postgresrepo.NewDispatchOfferRepositoryWithDB(
				tx,
			)

			current, err := offers.GetByIDForUpdate(
				ctx,
				id,
			)
			if err != nil {
				return err
			}

			if current.Status != StatusPending {
				return ErrOfferAlreadyResolved
			}

			now := time.Now().UTC()

			// If the offer has already expired, persist EXPIRED first.
			// The error is returned only after the transaction commits.
			if !now.Before(current.ExpiresAt) {

				if err := offers.UpdateStatus(
					ctx,
					current.ID,
					StatusExpired,
					&now,
					nil,
				); err != nil {
					return fmt.Errorf(
						"expire dispatch offer: %w",
						err,
					)
				}

				current.Status = StatusExpired
				current.RespondedAt = &now
				current.RejectionReason = nil
				current.UpdatedAt = now

				accepted = current
				wasExpired = true

				return nil
			}

			if err := offers.UpdateStatus(
				ctx,
				current.ID,
				StatusAccepted,
				&now,
				nil,
			); err != nil {
				return fmt.Errorf(
					"accept dispatch offer: %w",
					err,
				)
			}

			current.Status = StatusAccepted
			current.RespondedAt = &now
			current.RejectionReason = nil
			current.UpdatedAt = now

			accepted = current

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	if wasExpired {
		return accepted, ErrOfferExpired
	}

	return accepted, nil
}

// Reject transitions a PENDING dispatch offer to REJECTED.
//
// If the offer has already expired by time, it is transitioned to EXPIRED
// instead of REJECTED.
//
// When an offer becomes REJECTED or EXPIRED, its associated ride request
// is reset from MATCHING to PENDING inside the same PostgreSQL transaction.
// This makes the request eligible for another dispatch attempt.
func (s *Service) Reject(
	ctx context.Context,
	id string,
	reason string,
) (*models.DispatchOffer, error) {

	if id == "" {
		return nil, fmt.Errorf(
			"dispatch offer ID is required",
		)
	}

	if s.db == nil {
		return nil, fmt.Errorf(
			"dispatch offer database is not configured",
		)
	}

	var (
		rejected   *models.DispatchOffer
		wasExpired bool
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

			current, err := offers.GetByIDForUpdate(
				ctx,
				id,
			)
			if err != nil {
				return err
			}

			if current.Status != StatusPending {
				return ErrOfferAlreadyResolved
			}

			now := time.Now().UTC()

			// ---------------------------------------------------------
			// 2. If already expired, persist EXPIRED instead.
			// ---------------------------------------------------------

			if !now.Before(current.ExpiresAt) {

				if err := offers.UpdateStatus(
					ctx,
					current.ID,
					StatusExpired,
					&now,
					nil,
				); err != nil {
					return fmt.Errorf(
						"expire dispatch offer: %w",
						err,
					)
				}

				// Reset associated ride request so it can be dispatched again.
				if err := resetRideRequestAfterOfferResolution(
					ctx,
					rideRequests,
					current.RideRequestID,
					"expiry",
				); err != nil {
					return err
				}

				current.Status = StatusExpired
				current.RespondedAt = &now
				current.RejectionReason = nil
				current.UpdatedAt = now

				rejected = current
				wasExpired = true

				return nil
			}

			// ---------------------------------------------------------
			// 3. Build optional rejection reason.
			// ---------------------------------------------------------

			var rejectionReason *string

			if reason != "" {
				rejectionReason = &reason
			}

			// ---------------------------------------------------------
			// 4. Mark offer REJECTED.
			// ---------------------------------------------------------

			if err := offers.UpdateStatus(
				ctx,
				current.ID,
				StatusRejected,
				&now,
				rejectionReason,
			); err != nil {
				return fmt.Errorf(
					"reject dispatch offer: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 5. Reset MATCHING ride request back to PENDING.
			// ---------------------------------------------------------

			if err := resetRideRequestAfterOfferResolution(
				ctx,
				rideRequests,
				current.RideRequestID,
				"rejection",
			); err != nil {
				return err
			}

			current.Status = StatusRejected
			current.RespondedAt = &now
			current.RejectionReason = rejectionReason
			current.UpdatedAt = now

			rejected = current

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	if wasExpired {
		return rejected, ErrOfferExpired
	}

	return rejected, nil
}

// Expire transitions a PENDING dispatch offer to EXPIRED.
//
// When expiration succeeds, the associated ride request is reset from
// MATCHING to PENDING inside the same PostgreSQL transaction.
//
// It is safe to call this method from a future timeout worker or periodic
// expiration sweep.
func (s *Service) Expire(
	ctx context.Context,
	id string,
) (*models.DispatchOffer, error) {

	if id == "" {
		return nil, fmt.Errorf(
			"dispatch offer ID is required",
		)
	}

	if s.db == nil {
		return nil, fmt.Errorf(
			"dispatch offer database is not configured",
		)
	}

	var expired *models.DispatchOffer

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

			current, err := offers.GetByIDForUpdate(
				ctx,
				id,
			)
			if err != nil {

				if errors.Is(
					err,
					repository.ErrNotFound,
				) {
					return repository.ErrNotFound
				}

				return err
			}

			if current.Status != StatusPending {
				return ErrOfferAlreadyResolved
			}

			now := time.Now().UTC()

			// ---------------------------------------------------------
			// 2. Offer must actually be expired.
			// ---------------------------------------------------------

			if now.Before(current.ExpiresAt) {
				return fmt.Errorf(
					"dispatch offer has not expired yet",
				)
			}

			// ---------------------------------------------------------
			// 3. Mark dispatch offer EXPIRED.
			// ---------------------------------------------------------

			if err := offers.UpdateStatus(
				ctx,
				current.ID,
				StatusExpired,
				&now,
				nil,
			); err != nil {
				return fmt.Errorf(
					"expire dispatch offer: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 4. Reset MATCHING ride request back to PENDING.
			// ---------------------------------------------------------

			if err := resetRideRequestAfterOfferResolution(
				ctx,
				rideRequests,
				current.RideRequestID,
				"expiry",
			); err != nil {
				return err
			}

			current.Status = StatusExpired
			current.RespondedAt = &now
			current.RejectionReason = nil
			current.UpdatedAt = now

			expired = current

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return expired, nil
}

// resetRideRequestAfterOfferResolution returns a MATCHING ride request
// to PENDING after its dispatch offer is rejected or expires.
//
// Other lifecycle states are intentionally left unchanged. In particular,
// this helper must never move ACCEPTED, CANCELLED, or EXPIRED requests
// backwards into PENDING.
func resetRideRequestAfterOfferResolution(
	ctx context.Context,
	rideRequests repository.RideRequestRepository,
	rideRequestID string,
	reason string,
) error {

	if rideRequestID == "" {
		return fmt.Errorf(
			"ride request ID is required",
		)
	}

	request, err := rideRequests.GetByIDForUpdate(
		ctx,
		rideRequestID,
	)
	if err != nil {
		return fmt.Errorf(
			"get ride request after offer %s: %w",
			reason,
			err,
		)
	}

	if request.Status != "MATCHING" {
		return nil
	}

	if err := rideRequests.UpdateStatus(
		ctx,
		request.ID,
		"PENDING",
	); err != nil {
		return fmt.Errorf(
			"reset ride request after offer %s: %w",
			reason,
			err,
		)
	}

	return nil
}
