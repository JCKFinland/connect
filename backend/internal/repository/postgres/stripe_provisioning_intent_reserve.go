package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrStripeProvisioningIntentInvalid = errors.New(
		"invalid Stripe provisioning intent request",
	)
	ErrStripeProvisioningIntentConflict = errors.New(
		"Stripe provisioning intent identity conflict",
	)
)

type StripeProvisioningIntent struct {
	ID              string
	ActorUserID     string
	DriverUserID    string
	CompanyID       string
	IdempotencyKey  string
	Status          string
	StripeAccountID string
}

type ReserveStripeProvisioningIntentParams struct {
	ActorUserID  string
	DriverUserID string
	CompanyID    string
}

// ReserveStripeProvisioningIntent creates one durable PENDING intent
// per driver/company or returns the existing immutable reservation.
//
// Reservation is not authorization to call Stripe. A separate,
// authorized and fenced workflow must control any provider request.
//
// A replay never resets REQUESTED or REQUIRES_RECONCILIATION.
func ReserveStripeProvisioningIntent(
	ctx context.Context,
	db *pgxpool.Pool,
	params ReserveStripeProvisioningIntentParams,
) (*StripeProvisioningIntent, bool, error) {
	if ctx == nil || ctx.Err() != nil || db == nil ||
		!validProvisioningUUID(params.ActorUserID) ||
		!validProvisioningUUID(params.DriverUserID) ||
		!validProvisioningUUID(params.CompanyID) {
		return nil, false, ErrStripeProvisioningIntentInvalid
	}

	var result *StripeProvisioningIntent
	created := false

	err := RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		lockKey := "stripe-provisioning:" +
			params.DriverUserID + ":" + params.CompanyID

		if err := AcquireTransactionAdvisoryLock(
			ctx, tx, lockKey,
		); err != nil {
			return fmt.Errorf("lock provisioning identity: %w", err)
		}

		existing, err := readStripeProvisioningIntent(
			ctx, tx, params.DriverUserID, params.CompanyID,
		)

		if err == nil {
			if existing.ActorUserID != params.ActorUserID {
				return ErrStripeProvisioningIntentConflict
			}

			result = existing
			return nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read provisioning intent: %w", err)
		}

		// Generated only for a genuinely new reservation.
		// The database persists this key before any provider call.
		key := "connect-stripe-provisioning:" + uuid.NewString()

		result = &StripeProvisioningIntent{}

		const query = `
            INSERT INTO stripe_connect_provisioning_intents (
                actor_user_id,
                driver_user_id,
                company_id,
                idempotency_key
            ) VALUES (
                $1::uuid, $2::uuid, $3::uuid, $4
            )
            RETURNING
                id::text,
                actor_user_id::text,
                driver_user_id::text,
                company_id::text,
                idempotency_key,
                provisioning_status
        `

		err = tx.QueryRow(
			ctx, query,
			params.ActorUserID,
			params.DriverUserID,
			params.CompanyID,
			key,
		).Scan(
			&result.ID,
			&result.ActorUserID,
			&result.DriverUserID,
			&result.CompanyID,
			&result.IdempotencyKey,
			&result.Status,
		)

		if err != nil {
			return fmt.Errorf("insert provisioning intent: %w", err)
		}

		created = true
		return nil
	})

	if err != nil {
		return nil, false, err
	}

	return result, created, nil
}

func readStripeProvisioningIntent(
	ctx context.Context,
	tx pgx.Tx,
	driverID string,
	companyID string,
) (*StripeProvisioningIntent, error) {
	result := &StripeProvisioningIntent{}
	var accountID sql.NullString

	const query = `
        SELECT
            id::text,
            actor_user_id::text,
            driver_user_id::text,
            company_id::text,
            idempotency_key,
            provisioning_status,
            stripe_account_id
        FROM stripe_connect_provisioning_intents
        WHERE driver_user_id = $1::uuid
          AND company_id = $2::uuid
        FOR UPDATE
    `

	err := tx.QueryRow(
		ctx, query, driverID, companyID,
	).Scan(
		&result.ID,
		&result.ActorUserID,
		&result.DriverUserID,
		&result.CompanyID,
		&result.IdempotencyKey,
		&result.Status,
		&accountID,
	)

	if err != nil {
		return nil, err
	}

	if accountID.Valid {
		result.StripeAccountID = accountID.String
	}

	return result, nil
}

func validProvisioningUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil &&
		parsed != uuid.Nil &&
		parsed.String() == value
}
