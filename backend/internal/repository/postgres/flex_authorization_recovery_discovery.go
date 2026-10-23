package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultFlexAuthorizationRecoveryBatchSize = 25
	MaxFlexAuthorizationRecoveryBatchSize     = 100
)

// FlexAuthorizationRecoveryCandidate identifies an existing
// authorization eligible for retrieval-only provider recovery.
//
// Discovery does not reserve the candidate for execution.
type FlexAuthorizationRecoveryCandidate struct {
	AuthorizationID string
}

// DiscoverFlexAuthorizationRecoveryCandidates returns a bounded,
// deterministic list of stale PROCESSING authorizations whose
// Stripe PaymentIntent identity is already known.
//
// It never modifies financial records or contacts Stripe.
//
// expires_at is deliberately not used: it controls the initiation
// deadline, not the lifetime of an existing Stripe authorization.
func DiscoverFlexAuthorizationRecoveryCandidates(
	ctx context.Context,
	db *pgxpool.Pool,
	staleAfter time.Duration,
	batchSize int,
) ([]FlexAuthorizationRecoveryCandidate, error) {
	if ctx == nil {
		return nil, errors.New(
			"FLEX authorization recovery context is required",
		)
	}

	if db == nil {
		return nil, errors.New(
			"FLEX authorization recovery database is required",
		)
	}

	if staleAfter <= 0 {
		return nil, errors.New(
			"FLEX authorization recovery stale threshold must be positive",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if batchSize <= 0 {
		batchSize = DefaultFlexAuthorizationRecoveryBatchSize
	}

	if batchSize > MaxFlexAuthorizationRecoveryBatchSize {
		batchSize = MaxFlexAuthorizationRecoveryBatchSize
	}

	rows, err := db.Query(ctx, `
		SELECT id::text
		FROM flex_authorizations
		WHERE status = 'PROCESSING'
		  AND provider = 'STRIPE'
		  AND provider_authorization_id IS NOT NULL
		  AND btrim(provider_authorization_id) <> ''
		  AND updated_at <=
		      clock_timestamp() -
		      make_interval(
		          secs => ($1::double precision / 1000000.0)
		      )
		ORDER BY updated_at ASC, id ASC
		LIMIT $2
	`,
		float64(staleAfter)/float64(time.Microsecond),
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"discover FLEX authorization recovery candidates: %w",
			err,
		)
	}
	defer rows.Close()

	candidates := make(
		[]FlexAuthorizationRecoveryCandidate,
		0,
		batchSize,
	)

	for rows.Next() {
		var candidate FlexAuthorizationRecoveryCandidate

		if err := rows.Scan(&candidate.AuthorizationID); err != nil {
			return nil, fmt.Errorf(
				"scan FLEX authorization recovery candidate: %w",
				err,
			)
		}

		candidates = append(candidates, candidate)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate FLEX authorization recovery candidates: %w",
			err,
		)
	}

	return candidates, nil
}
