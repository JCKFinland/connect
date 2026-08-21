package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

const (
	defaultRedispatchWorkerInterval = 2 * time.Second
	defaultRedispatchBatchSize      = 100
)

// RedispatchWorkerOptions configures the automatic redispatch worker.
type RedispatchWorkerOptions struct {
	Interval  time.Duration
	BatchSize int

	// OnError receives non-fatal worker errors.
	//
	// The worker continues operating after these errors.
	OnError func(error)
}

// StartRedispatchWorker runs automatic dispatch recovery until ctx is
// cancelled.
//
// The worker is database-driven, so dispatch state survives application
// restarts and does not depend on per-offer in-memory timers.
func (s *Service) StartRedispatchWorker(
	ctx context.Context,
	options RedispatchWorkerOptions,
) {
	if s == nil || s.db == nil {
		return
	}

	interval := options.Interval
	if interval <= 0 {
		interval = defaultRedispatchWorkerInterval
	}

	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultRedispatchBatchSize
	}

	reportError := func(err error) {
		if err == nil {
			return
		}

		if options.OnError != nil {
			options.OnError(err)
		}
	}

	// Run once immediately during application startup.
	if err := s.runRedispatchCycle(
		ctx,
		batchSize,
	); err != nil && !errors.Is(err, context.Canceled) {
		reportError(err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {

		case <-ctx.Done():
			return

		case <-ticker.C:

			if err := s.runRedispatchCycle(
				ctx,
				batchSize,
			); err != nil {

				if errors.Is(
					err,
					context.Canceled,
				) {
					return
				}

				reportError(err)
			}
		}
	}
}

// runRedispatchCycle performs one worker cycle.
func (s *Service) runRedispatchCycle(
	ctx context.Context,
	batchSize int,
) error {

	// ---------------------------------------------------------
	// 1. Expire stale offers and recover their ride requests.
	// ---------------------------------------------------------

	if err := s.recoverExpiredDispatchOffers(
		ctx,
	); err != nil {
		return fmt.Errorf(
			"recover expired dispatch offers: %w",
			err,
		)
	}

	// ---------------------------------------------------------
	// 2. Find rides requiring another dispatch attempt.
	// ---------------------------------------------------------

	rideRequestIDs, err :=
		s.offers.ListRedispatchableRideRequestIDs(
			ctx,
			batchSize,
		)

	if err != nil {
		return fmt.Errorf(
			"list redispatchable rides: %w",
			err,
		)
	}

	// ---------------------------------------------------------
	// 3. Attempt redispatch.
	//
	// Failure to find a driver is not a worker failure.
	// The ride remains PENDING and will be retried next cycle.
	// ---------------------------------------------------------

	for _, rideRequestID := range rideRequestIDs {

		if rideRequestID == "" {
			continue
		}

		_, err := s.CreateOffer(
			ctx,
			rideRequestID,
			"", // worker-generated offer
		)

		if err == nil {
			continue
		}

		if errors.Is(
			err,
			ErrNoAvailableDrivers,
		) {
			continue
		}

		// A concurrent API dispatch may have claimed the ride between
		// discovery and CreateOffer(). Do not terminate the whole worker.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Report other failures but continue processing other rides.
		//
		// Returning one aggregate error would prevent remaining rides
		// from being processed, so report this cycle failure through
		// the caller one ride at a time.
		return fmt.Errorf(
			"redispatch ride request %s: %w",
			rideRequestID,
			err,
		)
	}

	return nil
}

// recoverExpiredDispatchOffers atomically:
//
//   - marks stale PENDING offers EXPIRED
//   - resets their MATCHING ride requests to PENDING
func (s *Service) recoverExpiredDispatchOffers(
	ctx context.Context,
) error {

	now := time.Now().UTC()

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			offers :=
				postgresrepo.NewDispatchOfferRepositoryWithDB(
					tx,
				)

			rideRequests :=
				postgresrepo.NewRideRequestRepositoryWithDB(
					tx,
				)

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

			for _, rideRequestID := range expiredRideRequestIDs {

				if rideRequestID == "" {
					continue
				}

				request, err :=
					rideRequests.GetByIDForUpdate(
						ctx,
						rideRequestID,
					)

				if err != nil {
					return fmt.Errorf(
						"get expired offer ride request: %w",
						err,
					)
				}

				if request.Status !=
					rideRequestStatusMatching {

					continue
				}

				if err :=
					rideRequests.UpdateStatus(
						ctx,
						request.ID,
						rideRequestStatusPending,
					); err != nil {

					return fmt.Errorf(
						"reset expired offer ride request: %w",
						err,
					)
				}
			}

			return nil
		},
	)
}
