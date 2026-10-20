package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FlexSettlementCategoryPage struct {
	Capture  []FlexSettlementCandidate
	Recovery []FlexSettlementCandidate
}

// DiscoverFlexSettlementCategoryPage returns independently ordered
// CAPTURE and RECOVERY candidate pages.
//
// Each cursor applies only to its corresponding category.
// Empty cursors begin at the oldest eligible settlement.
//
// The caller must wrap a category cursor to empty after that
// category returns no additional candidates.
//
// Discovery never claims settlements or contacts Stripe.
func DiscoverFlexSettlementCategoryPage(
	ctx context.Context,
	db *pgxpool.Pool,
	captureAfterID string,
	recoveryAfterID string,
	batchSize int,
) (FlexSettlementCategoryPage, error) {
	var page FlexSettlementCategoryPage

	if db == nil {
		return page, errors.New(
			"FLEX settlement database is required",
		)
	}

	if batchSize <= 0 {
		batchSize = DefaultFlexSettlementDiscoveryBatchSize
	}

	if batchSize > MaxFlexSettlementDiscoveryBatchSize {
		batchSize = MaxFlexSettlementDiscoveryBatchSize
	}

	captureLimit := (batchSize + 1) / 2
	recoveryLimit := batchSize / 2

	var err error

	page.Capture, err = discoverFlexSettlementCategory(
		ctx,
		db,
		FlexSettlementCandidateCapture,
		captureAfterID,
		captureLimit,
	)

	if err != nil {
		return FlexSettlementCategoryPage{}, err
	}

	page.Recovery, err = discoverFlexSettlementCategory(
		ctx,
		db,
		FlexSettlementCandidateRecovery,
		recoveryAfterID,
		recoveryLimit,
	)

	if err != nil {
		return FlexSettlementCategoryPage{}, err
	}

	// Redistribute unused reserved capacity without moving either
	// category cursor or selecting an earlier candidate twice.
	unusedCapture := captureLimit - len(page.Capture)
	unusedRecovery := recoveryLimit - len(page.Recovery)

	if unusedCapture > 0 && len(page.Recovery) == recoveryLimit {
		cursor := recoveryAfterID
		if len(page.Recovery) > 0 {
			cursor = page.Recovery[len(page.Recovery)-1].SettlementID
		}

		extra, err := discoverFlexSettlementCategory(
			ctx,
			db,
			FlexSettlementCandidateRecovery,
			cursor,
			unusedCapture,
		)
		if err != nil {
			return FlexSettlementCategoryPage{}, err
		}

		page.Recovery = append(page.Recovery, extra...)
	}

	if unusedRecovery > 0 && len(page.Capture) == captureLimit {
		cursor := captureAfterID
		if len(page.Capture) > 0 {
			cursor = page.Capture[len(page.Capture)-1].SettlementID
		}

		extra, err := discoverFlexSettlementCategory(
			ctx,
			db,
			FlexSettlementCandidateCapture,
			cursor,
			unusedRecovery,
		)
		if err != nil {
			return FlexSettlementCategoryPage{}, err
		}

		page.Capture = append(page.Capture, extra...)
	}

	return page, nil
}

// DiscoverFlexSettlementCategoryAfter retrieves candidates from
// exactly one settlement category using its independent cursor.
//
// It does not redistribute capacity to another category, claim
// settlements, modify financial records, or contact Stripe.
func DiscoverFlexSettlementCategoryAfter(
	ctx context.Context,
	db *pgxpool.Pool,
	kind FlexSettlementCandidateKind,
	afterID string,
	limit int,
) ([]FlexSettlementCandidate, error) {
	if db == nil {
		return nil, errors.New(
			"FLEX settlement database is required",
		)
	}

	if kind != FlexSettlementCandidateCapture &&
		kind != FlexSettlementCandidateRecovery {
		return nil, fmt.Errorf(
			"invalid FLEX settlement discovery category: %q",
			kind,
		)
	}

	if limit <= 0 {
		return nil, nil
	}

	if limit > MaxFlexSettlementDiscoveryBatchSize {
		limit = MaxFlexSettlementDiscoveryBatchSize
	}

	return discoverFlexSettlementCategory(
		ctx,
		db,
		kind,
		afterID,
		limit,
	)
}

func discoverFlexSettlementCategory(
	ctx context.Context,
	db *pgxpool.Pool,
	kind FlexSettlementCandidateKind,
	afterID string,
	limit int,
) ([]FlexSettlementCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}

	var cursorCreatedAt *string

	if afterID != "" {
		if _, err := uuid.Parse(afterID); err != nil {
			return nil, fmt.Errorf(
				"invalid FLEX settlement category cursor: %w",
				err,
			)
		}

		var timestamp string

		err := db.QueryRow(ctx, `
            SELECT created_at::text
            FROM flex_settlements
            WHERE id = $1
        `, afterID).Scan(&timestamp)

		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New(
				"FLEX settlement category cursor does not exist",
			)
		}

		if err != nil {
			return nil, fmt.Errorf(
				"resolve FLEX settlement category cursor: %w",
				err,
			)
		}

		cursorCreatedAt = &timestamp
	}

	rows, err := db.Query(ctx, `
        SELECT
            id::text
        FROM flex_settlements
        WHERE status IN ('PENDING', 'PROCESSING')
          AND (
              (
                  $1::text = 'CAPTURE'
                  AND status = 'PENDING'
                  AND capture_requested_at IS NULL
              )
              OR
              (
                  $1::text = 'RECOVERY'
                  AND (
                      status = 'PROCESSING'
                      OR capture_requested_at IS NOT NULL
                  )
              )
          )
          AND (
              $2::timestamptz IS NULL
              OR created_at > $2::timestamptz
              OR (
                  created_at = $2::timestamptz
                  AND id > $3::uuid
              )
          )
        ORDER BY created_at ASC, id ASC
        LIMIT $4
    `,
		string(kind),
		cursorCreatedAt,
		nullableFlexSettlementCursorID(afterID),
		limit,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"discover FLEX settlement category %s: %w",
			kind,
			err,
		)
	}

	defer rows.Close()

	candidates := make(
		[]FlexSettlementCandidate,
		0,
		limit,
	)

	for rows.Next() {
		var id string

		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf(
				"scan FLEX settlement category: %w",
				err,
			)
		}

		candidates = append(
			candidates,
			FlexSettlementCandidate{
				SettlementID: id,
				Kind:         kind,
			},
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate FLEX settlement category: %w",
			err,
		)
	}

	return candidates, nil
}

func nullableFlexSettlementCursorID(id string) any {
	if id == "" {
		return nil
	}

	return id
}
