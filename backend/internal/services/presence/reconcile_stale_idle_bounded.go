package presence

import (
	"context"
	"fmt"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

const defaultStaleIdleBatchSize = 100

// ReconcileStaleIdleBounded processes at most batchSize stale-idle
// candidates during one reconciliation cycle.
//
// Candidate discovery is read-only. Each candidate is rechecked inside
// a separate transaction after locking its permanent driver aggregate.
//
// Drivers locked by another transaction are skipped without waiting.
// A later worker cycle can reconsider those drivers.
//
// The returned count is the number of presence rows transitioned
// to OFFLINE, not the number of candidates inspected.
func (s *Service) ReconcileStaleIdleBounded(
	ctx context.Context,
	now time.Time,
	batchSize int,
) (int64, error) {
	if s == nil {
		return 0, fmt.Errorf("presence service is required")
	}
	if s.db == nil {
		return 0, fmt.Errorf("presence database is required")
	}
	if s.cfg == nil {
		return 0, fmt.Errorf("presence configuration is required")
	}
	if now.IsZero() {
		return 0, fmt.Errorf("reconciliation time is required")
	}
	if s.cfg.Presence.HeartbeatTimeout <= 0 {
		return 0, fmt.Errorf(
			"presence heartbeat timeout must be greater than zero",
		)
	}
	if batchSize <= 0 {
		return 0, fmt.Errorf(
			"stale presence batch size must be greater than zero",
		)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	staleBefore := now.UTC().Add(
		-s.cfg.Presence.HeartbeatTimeout,
	)

	// Discovery must not acquire driver or presence row locks.
	discoveryRepo := postgresrepo.NewDriverPresenceRepositoryWithDB(s.db)

	candidateLister, ok := discoveryRepo.(interface {
		ListStaleIdleCandidates(
			context.Context,
			time.Time,
			string,
			int,
		) ([]postgresrepo.StaleIdleDriverCandidate, error)
	})
	if !ok {
		return 0, fmt.Errorf(
			"presence repository does not support stale candidate discovery",
		)
	}

	// Only one cycle may advance this service's cursor at a time.
	s.staleIdleCursorMu.Lock()
	defer s.staleIdleCursorMu.Unlock()

	candidates, err := candidateLister.ListStaleIdleCandidates(
		ctx,
		staleBefore,
		s.staleIdleCursor,
		batchSize,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"discover stale idle drivers: %w",
			err,
		)
	}

	if len(candidates) == 0 && s.staleIdleCursor != "" {
		// End of the keyset: wrap and discover from the beginning.
		s.staleIdleCursor = ""

		candidates, err = candidateLister.ListStaleIdleCandidates(
			ctx,
			staleBefore,
			"",
			batchSize,
		)
		if err != nil {
			return 0, fmt.Errorf(
				"discover stale idle drivers after cursor wrap: %w",
				err,
			)
		}
	}

	if len(candidates) > 0 {
		// Advance even when every candidate is locked or no longer
		// eligible. This prevents locked drivers from starving later
		// candidates across reconciliation cycles.
		s.staleIdleCursor = candidates[len(candidates)-1].DriverID
	}

	var expiredCount int64

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return expiredCount, err
		}

		tx, err := s.db.Begin(ctx)
		if err != nil {
			return expiredCount, fmt.Errorf(
				"begin stale presence transaction: %w",
				err,
			)
		}

		driverRepo := postgresrepo.NewDriverRepositoryWithDB(tx)

		locked, err := driverRepo.TryLockByID(
			ctx,
			candidate.DriverID,
		)
		if err != nil {
			_ = tx.Rollback(context.Background())
			return expiredCount, fmt.Errorf(
				"lock stale driver %s: %w",
				candidate.DriverID,
				err,
			)
		}

		if !locked {
			if err := tx.Rollback(ctx); err != nil {
				return expiredCount, fmt.Errorf(
					"rollback skipped stale driver transaction: %w",
					err,
				)
			}
			continue
		}

		// ExpireStaleIdle performs its own eligibility check and
		// protects BUSY drivers and active trips.
		presenceRepo := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)

		transitioned, err := presenceRepo.ExpireStaleIdle(
			ctx,
			candidate.UserID,
			staleBefore,
		)
		if err != nil {
			_ = tx.Rollback(context.Background())
			return expiredCount, fmt.Errorf(
				"expire stale presence for driver %s: %w",
				candidate.DriverID,
				err,
			)
		}

		if err := tx.Commit(ctx); err != nil {
			return expiredCount, fmt.Errorf(
				"commit stale presence for driver %s: %w",
				candidate.DriverID,
				err,
			)
		}

		if transitioned {
			expiredCount++
		}
	}

	return expiredCount, nil
}
