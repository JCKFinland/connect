package dispatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	s.log.InfoContext(
		ctx,
		"redispatch worker started",
		slog.Duration("interval", interval),
		slog.Int("batch_size", batchSize),
	)

	reportError := func(err error) {
		if err == nil {
			return
		}

		s.log.ErrorContext(
			ctx,
			"redispatch worker cycle failed",
			slog.Any("error", err),
		)

		if options.OnError != nil {
			options.OnError(err)
		}
	}

	// Run one recovery cycle immediately on startup.
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

			s.log.Info(
				"redispatch worker stopped",
				slog.String(
					"reason",
					"context cancelled",
				),
			)

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

// runRedispatchCycle performs one automatic redispatch cycle.
func (s *Service) runRedispatchCycle(
	ctx context.Context,
	batchSize int,
) error {

	cycleStartedAt := time.Now()

	s.log.DebugContext(
		ctx,
		"redispatch worker cycle started",
		slog.Int("batch_size", batchSize),
	)

	// ---------------------------------------------------------
	// 1. Expire stale offers and recover their ride requests.
	// ---------------------------------------------------------

	expiredCount, err := s.recoverExpiredDispatchOffers(
		ctx,
	)
	if err != nil {
		return fmt.Errorf(
			"recover expired dispatch offers: %w",
			err,
		)
	}

	if expiredCount > 0 {
		s.log.InfoContext(
			ctx,
			"stale dispatch offers expired",
			slog.Int("expired_offer_count", expiredCount),
		)
	}

	// ---------------------------------------------------------
	// 2. Discover rides that need another dispatch attempt.
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

	if len(rideRequestIDs) == 0 {

		s.log.DebugContext(
			ctx,
			"redispatch worker cycle completed",
			slog.Int("expired_offer_count", expiredCount),
			slog.Int("redispatch_candidate_count", 0),
			slog.Int("redispatch_success_count", 0),
			slog.Int("no_driver_count", 0),
			slog.Duration(
				"duration",
				time.Since(cycleStartedAt),
			),
		)

		return nil
	}

	// ---------------------------------------------------------
	// 3. Attempt redispatch.
	//
	// CreateOffer() acquires the authoritative PostgreSQL
	// advisory lock for the ride request.
	// ---------------------------------------------------------

	successCount := 0
	noDriverCount := 0
	skippedCount := 0

	for _, rideRequestID := range rideRequestIDs {

		if rideRequestID == "" {
			continue
		}

		s.log.DebugContext(
			ctx,
			"redispatch attempt started",
			slog.String(
				"ride_request_id",
				rideRequestID,
			),
		)

		offer, err := s.CreateOffer(
			ctx,
			rideRequestID,
			"",
		)

		if err == nil {

			successCount++

			s.log.InfoContext(
				ctx,
				"ride redispatched successfully",
				slog.String(
					"ride_request_id",
					rideRequestID,
				),
				slog.String(
					"dispatch_offer_id",
					offer.ID,
				),
				slog.String(
					"driver_id",
					offer.DriverID,
				),
				slog.String(
					"vehicle_id",
					offer.VehicleID,
				),
				slog.Time(
					"expires_at",
					offer.ExpiresAt,
				),
			)

			continue
		}

		if errors.Is(
			err,
			ErrNoAvailableDrivers,
		) {

			noDriverCount++

			attemptedAt := time.Now().UTC()

			retryCount, nextAttemptAt, retryErr :=
				s.rideRequests.ScheduleDispatchRetry(
					ctx,
					rideRequestID,
					attemptedAt,
				)

			if retryErr != nil {

				if ctx.Err() != nil {
					return ctx.Err()
				}

				s.log.ErrorContext(
					ctx,
					"failed to schedule dispatch retry",
					slog.String(
						"ride_request_id",
						rideRequestID,
					),
					slog.Any(
						"error",
						retryErr,
					),
				)

				continue
			}

			s.log.InfoContext(
				ctx,
				"redispatch deferred: no eligible driver",
				slog.String(
					"ride_request_id",
					rideRequestID,
				),
				slog.Int(
					"retry_count",
					retryCount,
				),
				slog.Time(
					"next_dispatch_attempt_at",
					nextAttemptAt,
				),
			)

			continue
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Concurrent API/worker activity may have already handled the
		// ride after it was discovered. CreateOffer() safely validates
		// state after acquiring the ride-level advisory lock.
		skippedCount++

		s.log.DebugContext(
			ctx,
			"redispatch skipped",
			slog.String(
				"ride_request_id",
				rideRequestID,
			),
			slog.Any(
				"reason",
				err,
			),
		)
	}

	s.log.DebugContext(
		ctx,
		"redispatch worker cycle completed",
		slog.Int(
			"expired_offer_count",
			expiredCount,
		),
		slog.Int(
			"redispatch_candidate_count",
			len(rideRequestIDs),
		),
		slog.Int(
			"redispatch_success_count",
			successCount,
		),
		slog.Int(
			"no_driver_count",
			noDriverCount,
		),
		slog.Int(
			"skipped_count",
			skippedCount,
		),
		slog.Duration(
			"duration",
			time.Since(cycleStartedAt),
		),
	)

	return nil
}

// recoverExpiredDispatchOffers atomically:
//
//   - marks stale PENDING offers EXPIRED
//   - resets their MATCHING ride requests to PENDING
//
// The returned integer is the number of offers expired during this cycle.
func (s *Service) recoverExpiredDispatchOffers(
	ctx context.Context,
) (int, error) {

	now := time.Now().UTC()

	expiredCount := 0

	err := postgresrepo.RunInTransaction(
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

			expiredCount = len(
				expiredRideRequestIDs,
			)

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

				s.log.DebugContext(
					ctx,
					"ride request recovered after offer expiry",
					slog.String(
						"ride_request_id",
						request.ID,
					),
					slog.String(
						"previous_status",
						rideRequestStatusMatching,
					),
					slog.String(
						"new_status",
						rideRequestStatusPending,
					),
				)
			}

			return nil
		},
	)
	if err != nil {
		return 0, err
	}

	return expiredCount, nil
}
