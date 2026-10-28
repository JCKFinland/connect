package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ValidateDriverPayoutRecoveryLease checks whether the
// supplied fencing token still owns the payout recovery lease.
//
// This is read-only and never initiates or finalizes transfers.
// Database time is authoritative.
func ValidateDriverPayoutRecoveryLease(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
	token string,
	requiredRemaining time.Duration,
) (bool, error) {
	if ctx == nil ||
		db == nil ||
		uuid.Validate(payoutID) != nil ||
		uuid.Validate(token) != nil ||
		requiredRemaining <= 0 ||
		requiredRemaining > 5*time.Minute {
		return false, ErrDriverPayoutRecoveryLeaseInvalid
	}

	var valid bool

	err := db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1
            FROM driver_payout_recovery_leases
            WHERE payout_id = $1::uuid
              AND lease_token = $2::uuid
              AND lease_expires_at >=
                  clock_timestamp() +
                  ($3::bigint * interval '1 microsecond')
        )
    `,
		payoutID,
		token,
		requiredRemaining.Microseconds(),
	).Scan(&valid)

	if err != nil {
		return false, fmt.Errorf(
			"validate driver payout recovery lease: %w",
			err,
		)
	}

	return valid, nil
}
