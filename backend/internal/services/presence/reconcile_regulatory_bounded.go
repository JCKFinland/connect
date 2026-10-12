package presence

import (
	"context"
	"fmt"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

// ReconcileRegulatoryBounded takes non-compliant idle drivers offline.
// Candidate discovery is read-only; each candidate is rechecked under
// the permanent driver aggregate lock in its own transaction.
func (s *Service) ReconcileRegulatoryBounded(
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
	if s.complianceForDB == nil {
		return 0, fmt.Errorf("transactional compliance evaluator is required")
	}
	if now.IsZero() {
		return 0, fmt.Errorf("reconciliation time is required")
	}
	if batchSize <= 0 {
		return 0, fmt.Errorf("regulatory batch size must be greater than zero")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	s.regulatoryCursorMu.Lock()
	defer s.regulatoryCursorMu.Unlock()

	discoveryRepo := postgresrepo.NewDriverPresenceRepositoryWithDB(s.db)

	candidateLister, ok := discoveryRepo.(interface {
		ListRegulatoryCandidates(
			context.Context,
			time.Time,
			string,
			int,
		) ([]postgresrepo.RegulatoryDriverCandidate, error)
	})
	if !ok {
		return 0, fmt.Errorf(
			"presence repository does not support regulatory candidate discovery",
		)
	}

	candidates, err := candidateLister.ListRegulatoryCandidates(
		ctx,
		now,
		s.regulatoryCursor,
		batchSize,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"discover regulatory candidates: %w",
			err,
		)
	}

	// Wrap only when the current keyset page is exhausted.
	if len(candidates) == 0 && s.regulatoryCursor != "" {
		s.regulatoryCursor = ""

		candidates, err = candidateLister.ListRegulatoryCandidates(
			ctx,
			now,
			"",
			batchSize,
		)
		if err != nil {
			return 0, fmt.Errorf(
				"discover regulatory candidates after cursor wrap: %w",
				err,
			)
		}
	}

	// Advance even when a candidate's aggregate lock is unavailable.
	if len(candidates) > 0 {
		s.regulatoryCursor = candidates[len(candidates)-1].DriverID
	}

	var transitionedCount int64

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return transitionedCount, err
		}

		tx, err := s.db.Begin(ctx)
		if err != nil {
			return transitionedCount, fmt.Errorf(
				"begin regulatory reconciliation transaction: %w",
				err,
			)
		}

		drivers := postgresrepo.NewDriverRepositoryWithDB(tx)

		locked, err := drivers.TryLockByID(ctx, candidate.DriverID)
		if err != nil {
			_ = tx.Rollback(context.Background())
			return transitionedCount, fmt.Errorf(
				"lock regulatory driver %s: %w",
				candidate.DriverID,
				err,
			)
		}

		if !locked {
			if err := tx.Rollback(ctx); err != nil {
				return transitionedCount, fmt.Errorf(
					"rollback skipped regulatory driver: %w",
					err,
				)
			}
			continue
		}

		presence := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)

		current, err := presence.GetByDriverIDForUpdate(
			ctx,
			candidate.UserID,
		)
		if err != nil {
			_ = tx.Rollback(context.Background())
			return transitionedCount, fmt.Errorf(
				"lock regulatory driver presence %s: %w",
				candidate.DriverID,
				err,
			)
		}

		// Candidate discovery is a snapshot. The driver may have
		// gone offline or become BUSY before this transaction.
		if current == nil ||
			!current.IsOnline ||
			(current.AvailabilityStatus != StatusAvailable &&
				current.AvailabilityStatus != StatusBreak) {

			if err := tx.Rollback(ctx); err != nil {
				return transitionedCount, fmt.Errorf(
					"rollback inapplicable regulatory candidate: %w",
					err,
				)
			}
			continue
		}

		compliance := s.complianceForDB(tx)
		if compliance == nil {
			_ = tx.Rollback(context.Background())
			return transitionedCount, fmt.Errorf(
				"transactional compliance evaluator is unavailable",
			)
		}

		eligible, err := compliance.IsEligible(
			ctx,
			candidate.DriverID,
			now,
		)
		if err != nil {
			_ = tx.Rollback(context.Background())
			return transitionedCount, fmt.Errorf(
				"evaluate regulatory compliance for driver %s: %w",
				candidate.DriverID,
				err,
			)
		}

		var transitioned bool

		if !eligible {
			transitioned, err = presence.UpdateAvailabilityIfIdle(
				ctx,
				candidate.UserID,
				StatusOffline,
				false,
			)
			if err != nil {
				_ = tx.Rollback(context.Background())
				return transitionedCount, fmt.Errorf(
					"offline non-compliant driver %s: %w",
					candidate.DriverID,
					err,
				)
			}
		}

		if err := tx.Commit(ctx); err != nil {
			return transitionedCount, fmt.Errorf(
				"commit regulatory reconciliation for driver %s: %w",
				candidate.DriverID,
				err,
			)
		}

		if transitioned {
			transitionedCount++
		}
	}

	return transitionedCount, nil
}
