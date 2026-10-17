package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrFlexWebhookEventNotFound = errors.New(
	"FLEX webhook event not found",
)

const FlexWebhookProcessingLease = 5 * time.Minute

// ClaimFlexWebhookEventProcessing acquires a durable processing lease.
//
// A successful claim increments AttemptCount. That attempt number
// must be checked by the later completion operation to reject stale
// workers.
//
// The claim does not call Stripe or reconcile an authorization.
func ClaimFlexWebhookEventProcessing(
	ctx context.Context,
	db *pgxpool.Pool,
	eventID string,
) (*FlexWebhookEvent, bool, error) {
	if db == nil {
		return nil, false, ErrFlexWebhookEventInvalid
	}

	if _, err := uuid.Parse(eventID); err != nil {
		return nil, false, ErrFlexWebhookEventInvalid
	}

	var result *FlexWebhookEvent
	var claimed bool

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		current, err := scanFlexWebhookEvent(
			tx.QueryRow(ctx, `
				SELECT `+flexWebhookEventColumns+`
				FROM flex_webhook_events
				WHERE id = $1
				FOR UPDATE
			`, eventID),
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFlexWebhookEventNotFound
		}
		if err != nil {
			return fmt.Errorf(
				"lock FLEX webhook event: %w",
				err,
			)
		}

		switch current.Status {
		case "PROCESSED":
			result = current
			return nil

		case "PROCESSING":
			if current.ProcessingStartedAt != nil {
				var leaseValid bool

				err = tx.QueryRow(ctx, `
					SELECT
						processing_started_at >
							clock_timestamp() - INTERVAL '5 minutes'
					FROM flex_webhook_events
					WHERE id = $1
				`, current.ID).Scan(&leaseValid)

				if err != nil {
					return fmt.Errorf(
						"verify FLEX webhook claim lease: %w",
						err,
					)
				}

				if leaseValid {
					result = current
					return nil
				}
			}

		case "RECEIVED", "FAILED":
			// Eligible for a new processing attempt.

		default:
			return ErrFlexWebhookEventInvalid
		}

		result, err = scanFlexWebhookEvent(
			tx.QueryRow(ctx, `
				UPDATE flex_webhook_events
				SET
					status = 'PROCESSING',
					attempt_count = attempt_count + 1,
					processing_started_at = clock_timestamp(),
					processed_at = NULL,
					last_error = NULL,
					updated_at = clock_timestamp()
				WHERE id = $1
				RETURNING `+flexWebhookEventColumns,
				eventID,
			),
		)
		if err != nil {
			return fmt.Errorf(
				"claim FLEX webhook event: %w",
				err,
			)
		}

		claimed = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}

	return result, claimed, nil
}
