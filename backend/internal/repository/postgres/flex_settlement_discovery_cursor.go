package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DiscoverFlexSettlementCandidatesAfter discovers eligible FLEX
// settlements strictly after a settlement cursor.
//
// The cursor identifies a settlement by UUID. Its created_at value
// is resolved from PostgreSQL to preserve deterministic ordering.
//
// An empty cursor begins at the oldest eligible settlement.
//
// Discovery is read-only and never contacts Stripe.
//
// The caller is responsible for resetting the cursor to empty
// when no additional candidates remain.
func DiscoverFlexSettlementCandidatesAfter(
	ctx context.Context,
	db *pgxpool.Pool,
	afterSettlementID string,
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

	if afterSettlementID == "" {
		return DiscoverFlexSettlementCandidates(
			ctx,
			db,
			batchSize,
		)
	}

	if _, err := uuid.Parse(afterSettlementID); err != nil {
		return nil, fmt.Errorf(
			"invalid FLEX settlement discovery cursor: %w",
			err,
		)
	}

	var cursorCreatedAt string

	err := db.QueryRow(ctx, `
        SELECT created_at::text
        FROM flex_settlements
        WHERE id = $1
    `, afterSettlementID).Scan(&cursorCreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errors.New(
			"FLEX settlement discovery cursor does not exist",
		)
	}

	if err != nil {
		return nil, fmt.Errorf(
			"resolve FLEX settlement discovery cursor: %w",
			err,
		)
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
              AND (
                  created_at > $2::timestamptz
                  OR (
                      created_at = $2::timestamptz
                      AND id > $3::uuid
                  )
              )
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
                0 AS selection_priority
            FROM reserved

            UNION ALL

            SELECT
                id,
                created_at,
                candidate_kind,
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
    `,
		batchSize,
		cursorCreatedAt,
		afterSettlementID,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"discover FLEX settlements after cursor: %w",
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
				"scan FLEX settlement cursor candidate: %w",
				err,
			)
		}

		candidates = append(candidates, candidate)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate FLEX settlement cursor candidates: %w",
			err,
		)
	}

	return candidates, nil
}
