package flex_settlement

import (
	"context"
	"errors"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DiscoveryScheduler maintains independent cursors for settlement
// capture and recovery discovery.
//
// It is not a payment worker. It never claims settlements,
// changes financial records, or contacts a payment provider.
//
// Call Next serially; cursor state is not concurrency-safe.
// settlementCategoryQuery is the read-only repository boundary
// used by the scheduler. Tests may replace it with a deterministic
// implementation without creating financial transactions.
type settlementCategoryQuery func(
	ctx context.Context,
	db *pgxpool.Pool,
	kind postgresrepo.FlexSettlementCandidateKind,
	afterID string,
	limit int,
) ([]postgresrepo.FlexSettlementCandidate, error)

type DiscoveryScheduler struct {
	db             *pgxpool.Pool
	captureCursor  string
	recoveryCursor string
	batchSize      int
	query          settlementCategoryQuery

	// Alternate priority so a one-item batch cannot permanently
	// starve either settlement category.
	preferRecovery bool
}

func NewDiscoveryScheduler(
	db *pgxpool.Pool,
	batchSize int,
) (*DiscoveryScheduler, error) {
	if db == nil {
		return nil, errors.New(
			"FLEX settlement discovery database is required",
		)
	}

	if batchSize <= 0 {
		batchSize =
			postgresrepo.DefaultFlexSettlementDiscoveryBatchSize
	}

	if batchSize >
		postgresrepo.MaxFlexSettlementDiscoveryBatchSize {
		batchSize =
			postgresrepo.MaxFlexSettlementDiscoveryBatchSize
	}

	return &DiscoveryScheduler{
		db:        db,
		batchSize: batchSize,
		query:     postgresrepo.DiscoverFlexSettlementCategoryAfter,
	}, nil
}

// Next discovers a bounded page of CAPTURE and RECOVERY work.
//
// A category that reaches the end of its cursor range is
// restarted from the beginning. Already returned IDs are
// excluded from the current discovery cycle.
//
// The scheduler intentionally does not perform any settlement
// execution. Callers must use the existing guarded settlement
// orchestration for all financial operations.
// Next discovers a bounded batch of CAPTURE and RECOVERY candidates.
//
// Each category has an independent cursor. A category wraps only
// after the repository confirms that its current range is exhausted.
//
// Discovery is read-only. This method never claims a settlement,
// contacts Stripe, or modifies financial records.
//
// Call Next serially; scheduler cursor state is not concurrency-safe.
func (s *DiscoveryScheduler) Next(
	ctx context.Context,
) ([]postgresrepo.FlexSettlementCandidate, error) {
	if s == nil || s.db == nil {
		return nil, errors.New(
			"FLEX settlement discovery scheduler is required",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	captureLimit := (s.batchSize + 1) / 2
	recoveryLimit := s.batchSize / 2

	if s.preferRecovery {
		captureLimit = s.batchSize / 2
		recoveryLimit = (s.batchSize + 1) / 2
	}

	// Do not commit cursor changes until all database reads succeed.
	nextCaptureCursor := s.captureCursor
	nextRecoveryCursor := s.recoveryCursor

	fetch := func(
		kind postgresrepo.FlexSettlementCandidateKind,
		cursor *string,
		limit int,
		allowWrap bool,
	) ([]postgresrepo.FlexSettlementCandidate, error) {
		if limit <= 0 {
			return nil, nil
		}

		candidates, err :=
			s.query(
				ctx,
				s.db,
				kind,
				*cursor,
				limit,
			)

		if err != nil {
			return nil, err
		}

		// Wrap only when no candidates remain after the cursor.
		if allowWrap && len(candidates) == 0 && *cursor != "" {
			candidates, err =
				s.query(
					ctx,
					s.db,
					kind,
					"",
					limit,
				)

			if err != nil {
				return nil, err
			}

			// Reset the local cursor; the last returned candidate,
			// if any, becomes the next cursor below.
			*cursor = ""
		}

		if len(candidates) > 0 {
			*cursor = candidates[len(candidates)-1].SettlementID
		}

		return candidates, nil
	}

	capture, err := fetch(
		postgresrepo.FlexSettlementCandidateCapture,
		&nextCaptureCursor,
		captureLimit,
		true,
	)

	if err != nil {
		return nil, err
	}

	recovery, err := fetch(
		postgresrepo.FlexSettlementCandidateRecovery,
		&nextRecoveryCursor,
		recoveryLimit,
		true,
	)

	if err != nil {
		return nil, err
	}

	// Redistribute unused capacity without re-reading candidates
	// already returned from either category.
	unusedCapture := captureLimit - len(capture)
	unusedRecovery := recoveryLimit - len(recovery)

	if unusedCapture > 0 && len(recovery) == recoveryLimit {
		extra, err := fetch(
			postgresrepo.FlexSettlementCandidateRecovery,
			&nextRecoveryCursor,
			unusedCapture,
			false,
		)

		if err != nil {
			return nil, err
		}

		recovery = append(recovery, extra...)
	}

	if unusedRecovery > 0 && len(capture) == captureLimit {
		extra, err := fetch(
			postgresrepo.FlexSettlementCandidateCapture,
			&nextCaptureCursor,
			unusedRecovery,
			false,
		)

		if err != nil {
			return nil, err
		}

		capture = append(capture, extra...)
	}

	result := make(
		[]postgresrepo.FlexSettlementCandidate,
		0,
		s.batchSize,
	)

	result = append(result, capture...)
	result = append(result, recovery...)

	if len(result) > s.batchSize {
		return nil, errors.New(
			"FLEX discovery exceeded configured batch size",
		)
	}

	seen := make(map[string]struct{}, len(result))

	for _, candidate := range result {
		if _, exists := seen[candidate.SettlementID]; exists {
			return nil, errors.New(
				"FLEX discovery returned duplicate settlement",
			)
		}

		seen[candidate.SettlementID] = struct{}{}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Commit cursor changes only after the complete batch passes
	// validation.
	s.captureCursor = nextCaptureCursor
	s.recoveryCursor = nextRecoveryCursor
	s.preferRecovery = !s.preferRecovery

	return result, nil
}
