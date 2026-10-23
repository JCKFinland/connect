package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FlexAuthorizationHealthSnapshot reports outstanding authorization
// work without exposing passenger, provider, or financial identifiers.
type FlexAuthorizationHealthSnapshot struct {
	ObservedAt time.Time

	PendingCount                   int64
	ProcessingWithoutProviderCount int64
	ProcessingWithProviderCount    int64
	AuthorizedCount                int64

	StalePendingCount                   int64
	StaleProcessingWithoutProviderCount int64
	StaleProcessingWithProviderCount    int64

	OldestPendingAt                   *time.Time
	OldestProcessingWithoutProviderAt *time.Time
	OldestProcessingWithProviderAt    *time.Time
}

// ReadFlexAuthorizationHealthSnapshot executes a single read-only
// aggregate query. The threshold affects reporting only.
//
// expires_at is an initiation deadline, not evidence of a released
// Stripe payment authorization.
func ReadFlexAuthorizationHealthSnapshot(
	ctx context.Context,
	db *pgxpool.Pool,
	staleAfter time.Duration,
) (*FlexAuthorizationHealthSnapshot, error) {
	if ctx == nil {
		return nil, errors.New(
			"FLEX authorization health context is required",
		)
	}

	if db == nil {
		return nil, errors.New(
			"FLEX authorization health database is required",
		)
	}

	if staleAfter <= 0 {
		return nil, errors.New(
			"FLEX authorization health stale threshold must be positive",
		)
	}

	snapshot := &FlexAuthorizationHealthSnapshot{}

	const query = `
        WITH observation AS (
            SELECT clock_timestamp() AS observed_at
        )
        SELECT
            observation.observed_at,

            COUNT(*) FILTER (
                WHERE fa.status = 'PENDING'
            )::bigint,

            COUNT(*) FILTER (
                WHERE fa.status = 'PROCESSING'
                  AND fa.provider_authorization_id IS NULL
            )::bigint,

            COUNT(*) FILTER (
                WHERE fa.status = 'PROCESSING'
                  AND fa.provider_authorization_id IS NOT NULL
            )::bigint,

            COUNT(*) FILTER (
                WHERE fa.status = 'AUTHORIZED'
            )::bigint,

            COUNT(*) FILTER (
                WHERE fa.status = 'PENDING'
                  AND fa.created_at <=
                      observation.observed_at - make_interval(secs => ($1::double precision / 1000000.0))
            )::bigint,

            COUNT(*) FILTER (
                WHERE fa.status = 'PROCESSING'
                  AND fa.provider_authorization_id IS NULL
                  AND fa.updated_at <=
                      observation.observed_at - make_interval(secs => ($1::double precision / 1000000.0))
            )::bigint,

            COUNT(*) FILTER (
                WHERE fa.status = 'PROCESSING'
                  AND fa.provider_authorization_id IS NOT NULL
                  AND fa.updated_at <=
                      observation.observed_at - make_interval(secs => ($1::double precision / 1000000.0))
            )::bigint,

            MIN(fa.created_at) FILTER (
                WHERE fa.status = 'PENDING'
            ),

            MIN(fa.updated_at) FILTER (
                WHERE fa.status = 'PROCESSING'
                  AND fa.provider_authorization_id IS NULL
            ),

            MIN(fa.updated_at) FILTER (
                WHERE fa.status = 'PROCESSING'
                  AND fa.provider_authorization_id IS NOT NULL
            )

        FROM observation
        LEFT JOIN flex_authorizations fa ON TRUE
        GROUP BY observation.observed_at
    `

	err := db.QueryRow(
		ctx,
		query,
		float64(staleAfter)/float64(time.Microsecond),
	).Scan(
		&snapshot.ObservedAt,
		&snapshot.PendingCount,
		&snapshot.ProcessingWithoutProviderCount,
		&snapshot.ProcessingWithProviderCount,
		&snapshot.AuthorizedCount,
		&snapshot.StalePendingCount,
		&snapshot.StaleProcessingWithoutProviderCount,
		&snapshot.StaleProcessingWithProviderCount,
		&snapshot.OldestPendingAt,
		&snapshot.OldestProcessingWithoutProviderAt,
		&snapshot.OldestProcessingWithProviderAt,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"read FLEX authorization health snapshot: %w",
			err,
		)
	}

	return snapshot, nil
}
