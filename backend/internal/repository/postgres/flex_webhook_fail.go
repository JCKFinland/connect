package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FailFlexWebhookEvent releases an active processing claim after a
// recoverable processing failure.
//
// The attempt number fences out superseded workers. Expired leases,
// processed events, and claims owned by another attempt cannot be
// changed.
//
// lastError must be a fixed, non-sensitive diagnostic code, never
// a raw Stripe error, webhook payload, or payment credential.
func FailFlexWebhookEvent(
	ctx context.Context,
	db *pgxpool.Pool,
	eventID string,
	expectedAttempt int,
	lastError string,
) (*FlexWebhookEvent, error) {
	if db == nil || expectedAttempt < 1 {
		return nil, ErrFlexWebhookEventInvalid
	}

	if _, err := uuid.Parse(eventID); err != nil {
		return nil, ErrFlexWebhookEventInvalid
	}

	lastError = strings.TrimSpace(lastError)
	if lastError == "" || len(lastError) > 100 {
		return nil, ErrFlexWebhookEventInvalid
	}

	for _, r := range lastError {
		if !((r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '_') {
			return nil, ErrFlexWebhookEventInvalid
		}
	}

	var result *FlexWebhookEvent

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		var err error

		result, err = scanFlexWebhookEvent(
			tx.QueryRow(ctx, `
				UPDATE flex_webhook_events
				SET
					status = 'FAILED',
					last_error = $3,
					processing_started_at = NULL,
					updated_at = clock_timestamp()
				WHERE id = $1
				  AND status = 'PROCESSING'
				  AND attempt_count = $2
				  AND processing_started_at >
					  clock_timestamp() - INTERVAL '5 minutes'
				RETURNING `+flexWebhookEventColumns,
				eventID,
				expectedAttempt,
				lastError,
			),
		)

		if err == pgx.ErrNoRows {
			return ErrFlexWebhookClaimConflict
		}
		if err != nil {
			return fmt.Errorf(
				"mark FLEX webhook processing failed: %w",
				err,
			)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}
