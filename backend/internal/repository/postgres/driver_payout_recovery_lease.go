package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDriverPayoutRecoveryLeaseInvalid = errors.New(
	"invalid driver payout recovery lease",
)

// DriverPayoutRecoveryLease represents one exclusive,
// time-bounded ownership period for provider lookup.
//
// It does not authorize transfer initiation or payout finalization.
type DriverPayoutRecoveryLease struct {
	PayoutID     string
	Token        string
	Owner        string
	ExpiresAt    time.Time
	NextLookupAt time.Time
}

// AcquireDriverPayoutRecoveryLease atomically reserves
// a payout for lookup-only recovery.
//
// The cooldown begins at acquisition, not after lookup.
// Database time is authoritative.
//
// A fresh token fences every new ownership period.
func AcquireDriverPayoutRecoveryLease(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
	owner string,
	leaseDuration time.Duration,
	cooldown time.Duration,
) (*DriverPayoutRecoveryLease, bool, error) {
	if ctx == nil ||
		db == nil ||
		uuid.Validate(payoutID) != nil ||
		strings.TrimSpace(owner) == "" ||
		len(owner) > 200 ||
		leaseDuration <= 0 ||
		leaseDuration > 5*time.Minute ||
		cooldown < leaseDuration ||
		cooldown > 24*time.Hour {
		return nil, false, ErrDriverPayoutRecoveryLeaseInvalid
	}

	token := uuid.NewString()

	var result DriverPayoutRecoveryLease

	err := db.QueryRow(ctx, `
        INSERT INTO driver_payout_recovery_leases (
            payout_id,
            lease_token,
            lease_owner,
            lease_expires_at,
            next_lookup_at,
            last_lookup_at,
            lookup_count
        )
        SELECT
            p.id,
            $2::uuid,
            $3,
            clock_timestamp() + ($4::bigint * interval '1 microsecond'),
            clock_timestamp() + ($5::bigint * interval '1 microsecond'),
            clock_timestamp(),
            1
        FROM driver_payouts p
        WHERE p.id = $1::uuid
          AND p.payout_status = 'PENDING'
          AND p.payout_method = 'BANK_TRANSFER'
          AND EXISTS (
              SELECT 1
              FROM driver_payout_executions e
              JOIN driver_payout_destinations d
                ON d.id = e.destination_id
              WHERE e.payout_id = p.id
                AND e.execution_status IN (
                    'PROCESSING',
                    'REQUIRES_RECONCILIATION'
                )
                AND e.attempt_count = 1
                AND e.request_started_at IS NOT NULL
                AND e.provider_reference IS NULL
                AND e.provider_confirmed_at IS NULL
                AND e.provider = d.provider
                AND e.amount = p.net_amount
                AND e.currency = p.currency
                AND d.driver_id = p.driver_id
                AND d.company_id = p.company_id
          )
        ON CONFLICT (payout_id)
        DO UPDATE SET
            lease_token = EXCLUDED.lease_token,
            lease_owner = EXCLUDED.lease_owner,
            lease_expires_at = EXCLUDED.lease_expires_at,
            next_lookup_at = EXCLUDED.next_lookup_at,
            last_lookup_at = EXCLUDED.last_lookup_at,
            lookup_count =
                driver_payout_recovery_leases.lookup_count + 1,
            updated_at = clock_timestamp()
        WHERE
            driver_payout_recovery_leases.lease_expires_at
                <= clock_timestamp()
            AND driver_payout_recovery_leases.next_lookup_at
                <= clock_timestamp()
            AND EXISTS (
                SELECT 1
                FROM driver_payouts p
                JOIN driver_payout_executions e
                  ON e.payout_id = p.id
                JOIN driver_payout_destinations d
                  ON d.id = e.destination_id
                WHERE p.id =
                    driver_payout_recovery_leases.payout_id
                  AND p.payout_status = 'PENDING'
                  AND p.payout_method = 'BANK_TRANSFER'
                  AND e.execution_status IN (
                      'PROCESSING',
                      'REQUIRES_RECONCILIATION'
                  )
                  AND e.attempt_count = 1
                  AND e.request_started_at IS NOT NULL
                  AND e.provider_reference IS NULL
                  AND e.provider_confirmed_at IS NULL
                  AND e.provider = d.provider
                  AND e.amount = p.net_amount
                  AND e.currency = p.currency
                  AND d.driver_id = p.driver_id
                  AND d.company_id = p.company_id
            )
        RETURNING
            payout_id::text,
            lease_token::text,
            lease_owner,
            lease_expires_at,
            next_lookup_at
    `,
		payoutID,
		token,
		owner,
		leaseDuration.Microseconds(),
		cooldown.Microseconds(),
	).Scan(
		&result.PayoutID,
		&result.Token,
		&result.Owner,
		&result.ExpiresAt,
		&result.NextLookupAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}

		return nil, false, fmt.Errorf(
			"acquire driver payout recovery lease: %w",
			err,
		)
	}

	return &result, true, nil
}

// ReleaseDriverPayoutRecoveryLease relinquishes ownership
// without removing the durable cooldown.
//
// A stale token cannot release a newer lease.
func ReleaseDriverPayoutRecoveryLease(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
	token string,
) (bool, error) {
	if ctx == nil ||
		db == nil ||
		uuid.Validate(payoutID) != nil ||
		uuid.Validate(token) != nil {
		return false, ErrDriverPayoutRecoveryLeaseInvalid
	}

	tag, err := db.Exec(ctx, `
        UPDATE driver_payout_recovery_leases
        SET
            lease_expires_at = clock_timestamp(),
            updated_at = clock_timestamp()
        WHERE payout_id = $1::uuid
          AND lease_token = $2::uuid
          AND lease_expires_at > clock_timestamp()
    `, payoutID, token)

	if err != nil {
		return false, fmt.Errorf(
			"release driver payout recovery lease: %w",
			err,
		)
	}

	return tag.RowsAffected() == 1, nil
}
