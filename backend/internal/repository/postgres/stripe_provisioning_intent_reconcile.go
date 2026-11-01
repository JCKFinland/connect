package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MarkStripeProvisioningReconciliationParams identifies an existing
// claimed provisioning request using immutable server-side identity.
// The idempotency key must never be generated or replaced here.
type MarkStripeProvisioningReconciliationParams struct {
	IntentID       string
	ActorUserID    string
	DriverUserID   string
	CompanyID      string
	IdempotencyKey string
}

// MarkStripeProvisioningReconciliation marks a REQUESTED intent as
// REQUIRES_RECONCILIATION after an uncertain provider outcome.
//
// transitioned=true means this transaction newly committed the
// reconciliation state. A replay of REQUIRES_RECONCILIATION returns
// transitioned=false without changing the durable intent.
//
// PENDING cannot be reconciled, and ACCOUNT_RECORDED remains terminal.
// This operation never authorizes a Stripe provider request, performs
// provider I/O, or proves account ownership or payout eligibility.
//
// A trusted service must separately authorize the actor and decide
// whether the provider outcome is sufficiently uncertain to invoke it.
func MarkStripeProvisioningReconciliation(
	ctx context.Context,
	db *pgxpool.Pool,
	params MarkStripeProvisioningReconciliationParams,
) (*StripeProvisioningIntent, bool, error) {
	if ctx == nil || ctx.Err() != nil || db == nil ||
		!validProvisioningUUID(params.IntentID) ||
		!validProvisioningUUID(params.ActorUserID) ||
		!validProvisioningUUID(params.DriverUserID) ||
		!validProvisioningUUID(params.CompanyID) ||
		params.IdempotencyKey == "" ||
		params.IdempotencyKey != strings.TrimSpace(params.IdempotencyKey) {
		return nil, false, ErrStripeProvisioningIntentInvalid
	}

	var result *StripeProvisioningIntent
	transitioned := false

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		lockKey := "stripe-provisioning:" +
			params.DriverUserID + ":" + params.CompanyID

		if err := AcquireTransactionAdvisoryLock(
			ctx, tx, lockKey,
		); err != nil {
			return fmt.Errorf(
				"lock provisioning reconciliation: %w", err,
			)
		}

		existing, err := readStripeProvisioningIntent(
			ctx, tx, params.DriverUserID, params.CompanyID,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStripeProvisioningIntentConflict
		}
		if err != nil {
			return fmt.Errorf(
				"read provisioning reconciliation: %w", err,
			)
		}

		if existing.ID != params.IntentID ||
			existing.ActorUserID != params.ActorUserID ||
			existing.DriverUserID != params.DriverUserID ||
			existing.CompanyID != params.CompanyID ||
			existing.IdempotencyKey != params.IdempotencyKey {
			return ErrStripeProvisioningIntentConflict
		}

		if existing.Status == "REQUIRES_RECONCILIATION" {
			result = existing
			return nil
		}

		// A completed account cannot be reopened or marked uncertain.
		if existing.Status != "REQUESTED" {
			return ErrStripeProvisioningIntentConflict
		}

		const query = `
            UPDATE stripe_connect_provisioning_intents
            SET provisioning_status = 'REQUIRES_RECONCILIATION'
            WHERE id = $1::uuid
              AND actor_user_id = $2::uuid
              AND driver_user_id = $3::uuid
              AND company_id = $4::uuid
              AND idempotency_key = $5
              AND provisioning_status = 'REQUESTED'
              AND stripe_account_id IS NULL
            RETURNING
                id::text,
                actor_user_id::text,
                driver_user_id::text,
                company_id::text,
                idempotency_key,
                provisioning_status
        `

		updated := &StripeProvisioningIntent{}

		err = tx.QueryRow(
			ctx, query,
			params.IntentID,
			params.ActorUserID,
			params.DriverUserID,
			params.CompanyID,
			params.IdempotencyKey,
		).Scan(
			&updated.ID,
			&updated.ActorUserID,
			&updated.DriverUserID,
			&updated.CompanyID,
			&updated.IdempotencyKey,
			&updated.Status,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStripeProvisioningIntentConflict
		}
		if err != nil {
			return fmt.Errorf(
				"transition provisioning reconciliation: %w", err,
			)
		}

		if updated.Status != "REQUIRES_RECONCILIATION" ||
			updated.IdempotencyKey != params.IdempotencyKey {
			return ErrStripeProvisioningIntentConflict
		}

		result = updated
		transitioned = true
		return nil
	})

	if err != nil {
		return nil, false, err
	}

	// A new transition is reported only after successful commit.
	return result, transitioned, nil
}
