package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ClaimStripeProvisioningIntentParams identifies an existing durable
// reservation. The intent ID and idempotency key must come from
// trusted server-side reservation state, never client input.
type ClaimStripeProvisioningIntentParams struct {
	IntentID       string
	ActorUserID    string
	DriverUserID   string
	CompanyID      string
	IdempotencyKey string
}

// ClaimStripeProvisioningIntent atomically moves exactly one matching
// PENDING intent to REQUESTED.
//
// claimed=true means the transition committed successfully.
// claimed=false means no new provider request is authorized.
//
// A failed or ambiguous transaction must never grant permission.
// No Stripe provider call is performed by this function.
//
// This repository operation is NOT an authorization decision.
// A separate trusted service must validate the actor, company,
// driver and linking session before invoking it.
func ClaimStripeProvisioningIntent(
	ctx context.Context,
	db *pgxpool.Pool,
	params ClaimStripeProvisioningIntentParams,
) (*StripeProvisioningIntent, bool, error) {
	if ctx == nil || ctx.Err() != nil || db == nil ||
		!validProvisioningUUID(params.IntentID) ||
		!validProvisioningUUID(params.ActorUserID) ||
		!validProvisioningUUID(params.DriverUserID) ||
		!validProvisioningUUID(params.CompanyID) ||
		params.IdempotencyKey == "" {
		return nil, false, ErrStripeProvisioningIntentInvalid
	}

	var result *StripeProvisioningIntent
	claimed := false

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		lockKey := "stripe-provisioning:" +
			params.DriverUserID + ":" + params.CompanyID

		if err := AcquireTransactionAdvisoryLock(
			ctx, tx, lockKey,
		); err != nil {
			return fmt.Errorf(
				"lock provisioning claim: %w", err,
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
				"read provisioning claim: %w", err,
			)
		}

		if existing.ID != params.IntentID ||
			existing.ActorUserID != params.ActorUserID ||
			existing.DriverUserID != params.DriverUserID ||
			existing.CompanyID != params.CompanyID ||
			existing.IdempotencyKey != params.IdempotencyKey {
			return ErrStripeProvisioningIntentConflict
		}

		result = existing

		// Replays never authorize another provider request.
		if existing.Status != "PENDING" {
			return nil
		}

		const query = `
            UPDATE stripe_connect_provisioning_intents
            SET provisioning_status = 'REQUESTED',
                request_started_at = clock_timestamp()
            WHERE id = $1::uuid
              AND actor_user_id = $2::uuid
              AND driver_user_id = $3::uuid
              AND company_id = $4::uuid
              AND idempotency_key = $5
              AND provisioning_status = 'PENDING'
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
				"transition provisioning claim: %w", err,
			)
		}

		if updated.Status != "REQUESTED" {
			return ErrStripeProvisioningIntentConflict
		}

		result = updated
		claimed = true
		return nil
	})

	if err != nil {
		return nil, false, err
	}

	// Permission is exposed only after transaction commit.
	return result, claimed, nil
}
