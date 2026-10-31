package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var stripeProvisioningAccountPattern = regexp.MustCompile(`^acct_[A-Za-z0-9]{8,64}$`)

// RecordStripeProvisioningAccountParams identifies one claimed
// operation and its trusted, provider-validated account result.
//
// All identities and the idempotency key must come from trusted
// server-side state, never directly from client input.
//
// This repository function does not verify Stripe ownership,
// onboarding, destination eligibility, or payout authority.
type RecordStripeProvisioningAccountParams struct {
	IntentID        string
	ActorUserID     string
	DriverUserID    string
	CompanyID       string
	IdempotencyKey  string
	StripeAccountID string
}

// RecordStripeProvisioningAccount persists one trusted provider
// result. recorded=true means this transaction newly recorded it.
//
// A replay with the same account ID is harmless and returns
// recorded=false. A different account ID is always a conflict.
//
// REQUESTED and REQUIRES_RECONCILIATION may be finalized only
// with trusted account evidence. This function does not fetch,
// create, or independently verify any Stripe account.
//
// Callers must separately enforce authorization and validate
// provider evidence before invoking this operation.
func RecordStripeProvisioningAccount(
	ctx context.Context,
	db *pgxpool.Pool,
	params RecordStripeProvisioningAccountParams,
) (*StripeProvisioningIntent, bool, error) {
	if ctx == nil || ctx.Err() != nil || db == nil ||
		!validProvisioningUUID(params.IntentID) ||
		!validProvisioningUUID(params.ActorUserID) ||
		!validProvisioningUUID(params.DriverUserID) ||
		!validProvisioningUUID(params.CompanyID) ||
		params.IdempotencyKey == "" ||
		params.IdempotencyKey !=
			trimProvisioningKey(params.IdempotencyKey) ||
		!stripeProvisioningAccountPattern.MatchString(
			params.StripeAccountID,
		) {
		return nil, false, ErrStripeProvisioningIntentInvalid
	}

	var result *StripeProvisioningIntent
	recorded := false

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		lockKey := "stripe-provisioning:" +
			params.DriverUserID + ":" + params.CompanyID

		if err := AcquireTransactionAdvisoryLock(
			ctx, tx, lockKey,
		); err != nil {
			return fmt.Errorf(
				"lock provisioning record: %w", err,
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
				"read provisioning record: %w", err,
			)
		}

		if existing.ID != params.IntentID ||
			existing.ActorUserID != params.ActorUserID ||
			existing.DriverUserID != params.DriverUserID ||
			existing.CompanyID != params.CompanyID ||
			existing.IdempotencyKey != params.IdempotencyKey {
			return ErrStripeProvisioningIntentConflict
		}

		if existing.Status == "ACCOUNT_RECORDED" {
			if existing.StripeAccountID != params.StripeAccountID {
				return ErrStripeProvisioningIntentConflict
			}
			result = existing
			return nil
		}

		if existing.Status != "REQUESTED" &&
			existing.Status != "REQUIRES_RECONCILIATION" {
			return ErrStripeProvisioningIntentConflict
		}

		const query = `
            UPDATE stripe_connect_provisioning_intents
            SET provisioning_status = 'ACCOUNT_RECORDED',
                stripe_account_id = $6,
                account_recorded_at = clock_timestamp()
            WHERE id = $1::uuid
              AND actor_user_id = $2::uuid
              AND driver_user_id = $3::uuid
              AND company_id = $4::uuid
              AND idempotency_key = $5
              AND provisioning_status IN (
                  'REQUESTED',
                  'REQUIRES_RECONCILIATION'
              )
              AND stripe_account_id IS NULL
            RETURNING
                id::text,
                actor_user_id::text,
                driver_user_id::text,
                company_id::text,
                idempotency_key,
                provisioning_status,
                stripe_account_id
        `

		updated := &StripeProvisioningIntent{}

		err = tx.QueryRow(
			ctx, query,
			params.IntentID,
			params.ActorUserID,
			params.DriverUserID,
			params.CompanyID,
			params.IdempotencyKey,
			params.StripeAccountID,
		).Scan(
			&updated.ID,
			&updated.ActorUserID,
			&updated.DriverUserID,
			&updated.CompanyID,
			&updated.IdempotencyKey,
			&updated.Status,
			&updated.StripeAccountID,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStripeProvisioningIntentConflict
		}
		if err != nil {
			return fmt.Errorf(
				"persist provisioning account: %w", err,
			)
		}

		if updated.Status != "ACCOUNT_RECORDED" ||
			updated.StripeAccountID != params.StripeAccountID {
			return ErrStripeProvisioningIntentConflict
		}

		result = updated
		recorded = true
		return nil
	})

	if err != nil {
		return nil, false, err
	}

	// Only report a new recording after successful commit.
	return result, recorded, nil
}

func trimProvisioningKey(value string) string {
	return strings.TrimSpace(value)
}
