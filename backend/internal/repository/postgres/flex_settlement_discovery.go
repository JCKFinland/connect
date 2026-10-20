package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultFlexSettlementDiscoveryBatchSize = 50
	MaxFlexSettlementDiscoveryBatchSize     = 200
)

type FlexSettlementCandidateKind string

const (
	FlexSettlementCandidateCapture  FlexSettlementCandidateKind = "CAPTURE"
	FlexSettlementCandidateRecovery FlexSettlementCandidateKind = "RECOVERY"
)

type FlexSettlementCandidate struct {
	SettlementID string
	Kind         FlexSettlementCandidateKind
}

// DiscoverFlexSettlementCandidates returns a bounded, deterministic,
// read-only list of settlements requiring capture or reconciliation.
//
// This function does not claim settlements, update financial records,
// or contact Stripe. All state transitions remain the responsibility
// of the existing settlement orchestration services.
//
// PROCESSING settlements and settlements with an existing capture
// request are classified exclusively as RECOVERY candidates.
func DiscoverFlexSettlementCandidates(
	ctx context.Context,
	db *pgxpool.Pool,
	batchSize int,
) ([]FlexSettlementCandidate, error) {
	if db == nil {
		return nil, errors.New(
			"FLEX settlement database is required",
		)
	}

	if batchSize <= 0 {
		batchSize = DefaultFlexSettlementDiscoveryBatchSize
	}
	if batchSize > MaxFlexSettlementDiscoveryBatchSize {
		batchSize = MaxFlexSettlementDiscoveryBatchSize
	}

	rows, err := db.Query(ctx, `
        WITH classified AS (
            SELECT
                id,
                created_at,
                CASE
                    WHEN status = 'PENDING'
                         AND capture_requested_at IS NULL
                        THEN 'CAPTURE'
                    ELSE 'RECOVERY'
                END AS candidate_kind
            FROM flex_settlements
            WHERE status IN ('PENDING', 'PROCESSING')
        ),
        ranked AS (
            SELECT
                id,
                created_at,
                candidate_kind,
                ROW_NUMBER() OVER (
                    PARTITION BY candidate_kind
                    ORDER BY created_at ASC, id ASC
                ) AS category_rank
            FROM classified
        ),
        reserved AS (
            SELECT
                id,
                created_at,
                candidate_kind,
                category_rank
            FROM ranked
            WHERE
                (
                    candidate_kind = 'CAPTURE'
                    AND category_rank <= (($1::integer + 1) / 2)
                )
                OR
                (
                    candidate_kind = 'RECOVERY'
                    AND category_rank <= ($1::integer / 2)
                )
        ),
        remaining AS (
            SELECT
                id,
                created_at,
                candidate_kind,
                category_rank
            FROM ranked
            WHERE NOT EXISTS (
                SELECT 1
                FROM reserved
                WHERE reserved.id = ranked.id
            )
        ),
        selected AS (
            SELECT
                id,
                created_at,
                candidate_kind,
                category_rank,
                0 AS selection_priority
            FROM reserved

            UNION ALL

            SELECT
                id,
                created_at,
                candidate_kind,
                category_rank,
                1 AS selection_priority
            FROM remaining
            ORDER BY
                selection_priority,
                created_at,
                id
            LIMIT $1
        )
        SELECT
            id::text,
            candidate_kind
        FROM selected
        ORDER BY
            created_at ASC,
            id ASC
    `, batchSize)
	if err != nil {
		return nil, fmt.Errorf(
			"discover FLEX settlement candidates: %w",
			err,
		)
	}
	defer rows.Close()

	candidates := make(
		[]FlexSettlementCandidate,
		0,
		batchSize,
	)

	for rows.Next() {
		var candidate FlexSettlementCandidate

		if err := rows.Scan(
			&candidate.SettlementID,
			&candidate.Kind,
		); err != nil {
			return nil, fmt.Errorf(
				"scan FLEX settlement candidate: %w",
				err,
			)
		}

		candidates = append(candidates, candidate)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate FLEX settlement candidates: %w",
			err,
		)
	}

	return candidates, nil
}
