package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrFlexWebhookClaimConflict = errors.New(
	"FLEX webhook processing claim conflict",
)

// CompleteFlexWebhookEvent atomically reconciles an authorization
// and marks its verified webhook event PROCESSED.
//
// The caller must first validate the Stripe signature, financial
// identity, PaymentIntent state, and authorization outcome.
//
// expectedAttempt is a fencing token obtained from a successful
// ClaimFlexWebhookEventProcessing call.
//
// No Stripe network request occurs inside this transaction.
func CompleteFlexWebhookEvent(
	ctx context.Context,
	db *pgxpool.Pool,
	eventID string,
	expectedAttempt int,
	providerAuthorizationID string,
	resultStatus FlexAuthorizationResultStatus,
) (*FlexAuthorization, error) {
	if db == nil || expectedAttempt < 1 {
		return nil, ErrFlexWebhookEventInvalid
	}

	if _, err := uuid.Parse(eventID); err != nil {
		return nil, ErrFlexWebhookEventInvalid
	}

	var authorization *FlexAuthorization

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		event, err := scanFlexWebhookEvent(
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
				"lock FLEX webhook event for completion: %w",
				err,
			)
		}

		if event.Status != "PROCESSING" ||
			event.AttemptCount != expectedAttempt ||
			event.ProcessingStartedAt == nil {
			return ErrFlexWebhookClaimConflict
		}

		// Check lease validity using the PostgreSQL clock.
		//
		// A worker whose lease has expired must not complete the event,
		// even if no replacement worker has claimed it yet.
		var leaseValid bool

		err = tx.QueryRow(ctx, `
	SELECT
		processing_started_at >
			clock_timestamp() - INTERVAL '5 minutes'
	FROM flex_webhook_events
	WHERE id = $1
`, event.ID).Scan(&leaseValid)
		if err != nil {
			return fmt.Errorf(
				"verify FLEX webhook processing lease: %w",
				err,
			)
		}

		if !leaseValid {
			return ErrFlexWebhookClaimConflict
		}

		if event.Provider != "STRIPE" ||
			event.ProviderAuthorizationID != providerAuthorizationID {
			return ErrFlexWebhookEventIdentityConflict
		}

		// Reconcile inside this same transaction. Any subsequent
		// failure rolls back the authorization transition.
		authorization, err = ReconcileFlexAuthorizationTx(
			ctx,
			tx,
			event.AuthorizationID,
			providerAuthorizationID,
			resultStatus,
		)
		if err != nil {
			return fmt.Errorf(
				"reconcile FLEX webhook authorization: %w",
				err,
			)
		}

		_, err = tx.Exec(ctx, `
			UPDATE flex_webhook_events
			SET
				status = 'PROCESSED',
				processed_at = clock_timestamp(),
				updated_at = clock_timestamp(),
				last_error = NULL
			WHERE id = $1
		`, event.ID)
		if err != nil {
			return fmt.Errorf(
				"complete FLEX webhook event: %w",
				err,
			)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return authorization, nil
}
