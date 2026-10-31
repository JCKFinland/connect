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

// FinalizeDriverPayoutExecutionSuccess records an externally verified
// successful provider payout.
//
// The caller must independently verify the provider's successful
// transfer, payout identity, amount, currency and provider reference
// before invoking this function.
//
// This function does not contact any external provider or move money.
//
// The execution, parent payout and claimed driver earnings are
// finalized in a single database transaction.
func FinalizeDriverPayoutExecutionSuccess(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
	provider string,
	providerReference string,
) error {
	return finalizeDriverPayoutExecutionSuccess(
		ctx, db, payoutID, provider, providerReference, "",
	)
}

// FinalizeDriverPayoutRecoverySuccess requires an active recovery lease.
// The lease row is locked inside the financial transaction.
func FinalizeDriverPayoutRecoverySuccess(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
	provider string,
	providerReference string,
	leaseToken string,
) error {
	if uuid.Validate(leaseToken) != nil {
		return ErrDriverPayoutRecoveryLeaseInvalid
	}
	return finalizeDriverPayoutExecutionSuccess(
		ctx, db, payoutID, provider, providerReference, leaseToken,
	)
}

func finalizeDriverPayoutExecutionSuccess(
	ctx context.Context,
	db *pgxpool.Pool,
	payoutID string,
	provider string,
	providerReference string,
	leaseToken string,
) error {
	if db == nil {
		return ErrDriverPayoutExecutionInvalidInput
	}

	parsedID, err := uuid.Parse(payoutID)
	if err != nil {
		return ErrDriverPayoutExecutionInvalidInput
	}

	provider = strings.TrimSpace(provider)
	providerReference = strings.TrimSpace(providerReference)

	if provider == "" ||
		len(provider) > 50 ||
		provider != strings.ToUpper(provider) ||
		providerReference == "" ||
		len(providerReference) > 255 {
		return ErrDriverPayoutExecutionInvalidInput
	}

	return RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		if leaseToken != "" {
			var lockedToken string
			err := tx.QueryRow(ctx, `
				SELECT lease_token::text
				FROM driver_payout_recovery_leases
				WHERE payout_id = $1::uuid
				  AND lease_token = $2::uuid
				  AND lease_expires_at >
				      clock_timestamp() +
				      ($3::bigint * interval '1 microsecond')
				FOR UPDATE
			`, payoutID, leaseToken,
				(5 * time.Second).Microseconds(),
			).Scan(&lockedToken)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrDriverPayoutRecoveryLeaseInvalid
			}
			if err != nil {
				return fmt.Errorf(
					"lock recovery finalization lease: %w", err,
				)
			}
			if lockedToken != leaseToken {
				return ErrDriverPayoutRecoveryLeaseInvalid
			}
		}

		var driverID, companyID string
		var amount, currency, payoutStatus, method string
		var payoutProvider, payoutProviderReference *string
		var processedAtPresent bool

		// Global lock order: parent payout before execution.
		err := tx.QueryRow(ctx, `
            SELECT
                driver_id,
                company_id,
                net_amount::text,
                currency,
                payout_status,
                payout_method,
                provider,
                provider_reference,
                processed_at IS NOT NULL
            FROM driver_payouts
            WHERE id = $1
            FOR UPDATE
        `, parsedID).Scan(
			&driverID,
			&companyID,
			&amount,
			&currency,
			&payoutStatus,
			&method,
			&payoutProvider,
			&payoutProviderReference,
			&processedAtPresent,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("lock payout for finalization: %w", err)
		}

		var executionProvider, executionAmount string
		var executionCurrency, executionStatus string
		var executionReference *string
		var requestStarted, confirmed bool
		var attemptCount int

		err = tx.QueryRow(ctx, `
            SELECT
                provider,
                amount::text,
                currency,
                execution_status,
                provider_reference,
                request_started_at IS NOT NULL,
                provider_confirmed_at IS NOT NULL,
                attempt_count
            FROM driver_payout_executions
            WHERE payout_id = $1
            FOR UPDATE
        `, parsedID).Scan(
			&executionProvider,
			&executionAmount,
			&executionCurrency,
			&executionStatus,
			&executionReference,
			&requestStarted,
			&confirmed,
			&attemptCount,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("lock payout execution: %w", err)
		}

		if method != "BANK_TRANSFER" ||
			currency != "EUR" ||
			executionCurrency != currency ||
			executionProvider != provider ||
			!requestStarted ||
			attemptCount != 1 {
			return ErrDriverPayoutExecutionConflict
		}

		// Exact NUMERIC comparison; never compare money as text.
		var amountMatches bool

		err = tx.QueryRow(ctx, `
            SELECT amount = $2::numeric
            FROM driver_payout_executions
            WHERE payout_id = $1
        `, parsedID, amount).Scan(&amountMatches)

		if err != nil {
			return fmt.Errorf(
				"verify execution amount: %w", err,
			)
		}

		if !amountMatches {
			return ErrDriverPayoutExecutionConflict
		}

		_ = executionAmount

		// Exact successful replay must be read-only.
		if executionStatus == "SUCCEEDED" {
			if executionReference == nil ||
				*executionReference != providerReference ||
				!confirmed ||
				payoutStatus != "PAID" ||
				payoutProvider == nil ||
				*payoutProvider != provider ||
				payoutProviderReference == nil ||
				*payoutProviderReference != providerReference ||
				!processedAtPresent {
				return ErrDriverPayoutExecutionConflict
			}

			var totalItems, unpaidItems int64
			err := tx.QueryRow(ctx, `
                SELECT
                    COUNT(*),
                    COUNT(*) FILTER (
                        WHERE e.settlement_status <> 'PAID'
                           OR e.settled_at IS NULL
                    )
                FROM driver_payout_items i
                JOIN driver_earnings e ON e.id = i.earning_id
                WHERE i.payout_id = $1
            `, parsedID).Scan(&totalItems, &unpaidItems)

			if err != nil {
				return fmt.Errorf("verify finalized earnings: %w", err)
			}

			if totalItems == 0 || unpaidItems != 0 {
				return ErrDriverPayoutExecutionConflict
			}

			return nil
		}

		if payoutStatus != "PENDING" ||
			processedAtPresent ||
			payoutProvider != nil ||
			payoutProviderReference != nil ||
			confirmed ||
			executionReference != nil ||
			(executionStatus != "PROCESSING" &&
				executionStatus != "REQUIRES_RECONCILIATION") {
			return ErrDriverPayoutExecutionConflict
		}

		// Lock every claimed earning in a deterministic order.
		rows, err := tx.Query(ctx, `
            SELECT
                i.earning_id,
                i.amount::text,
                e.net_amount::text,
                e.driver_id,
                e.company_id,
                e.currency,
                e.settlement_status,
                e.settled_at IS NOT NULL
            FROM driver_payout_items i
            JOIN driver_earnings e ON e.id = i.earning_id
            WHERE i.payout_id = $1
            ORDER BY i.earning_id
            FOR UPDATE OF i, e
        `, parsedID)

		if err != nil {
			return fmt.Errorf("lock payout earnings: %w", err)
		}

		itemCount := 0

		for rows.Next() {
			var earningID, itemAmount, earningAmount string
			var earningDriver, earningCompany, earningCurrency string
			var earningStatus string
			var earningSettled bool

			if err := rows.Scan(
				&earningID,
				&itemAmount,
				&earningAmount,
				&earningDriver,
				&earningCompany,
				&earningCurrency,
				&earningStatus,
				&earningSettled,
			); err != nil {
				rows.Close()
				return fmt.Errorf("read payout earning: %w", err)
			}

			if earningDriver != driverID ||
				earningCompany != companyID ||
				earningCurrency != currency ||
				earningStatus != "PROCESSING" ||
				earningSettled {
				rows.Close()
				return ErrDriverPayoutExecutionConflict
			}

			_ = itemAmount
			_ = earningAmount
			itemCount++
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("read payout earning rows: %w", err)
		}

		rows.Close()

		if itemCount == 0 {
			return ErrDriverPayoutExecutionConflict
		}

		// PostgreSQL NUMERIC arithmetic avoids floating-point money math.
		var reconciled bool

		err = tx.QueryRow(ctx, `
            SELECT
                COALESCE(BOOL_AND(
                    i.amount = e.net_amount
                ), FALSE)
                AND COALESCE(SUM(i.amount), 0) = p.net_amount
                AND COALESCE(SUM(e.gross_amount), 0)
                    = p.gross_amount
                AND p.net_amount
                    = p.gross_amount - p.deductions + p.adjustments
            FROM driver_payouts p
            JOIN driver_payout_items i ON i.payout_id = p.id
            JOIN driver_earnings e ON e.id = i.earning_id
            WHERE p.id = $1
            GROUP BY
                p.id,
                p.net_amount,
                p.gross_amount,
                p.deductions,
                p.adjustments
        `, parsedID).Scan(&reconciled)

		if errors.Is(err, pgx.ErrNoRows) || !reconciled {
			return ErrDriverPayoutExecutionConflict
		}
		if err != nil {
			return fmt.Errorf("reconcile payout amounts: %w", err)
		}

		// All three financial state transitions share one transaction.
		tag, err := tx.Exec(ctx, `
            UPDATE driver_payout_executions
            SET execution_status = 'SUCCEEDED',
                provider_reference = $2,
                provider_confirmed_at = clock_timestamp(),
                last_error = NULL,
                updated_at = clock_timestamp()
            WHERE payout_id = $1
              AND execution_status IN (
                  'PROCESSING',
                  'REQUIRES_RECONCILIATION'
              )
              AND provider_reference IS NULL
              AND provider_confirmed_at IS NULL
              AND request_started_at IS NOT NULL
              AND attempt_count = 1
        `, parsedID, providerReference)

		if err != nil {
			return fmt.Errorf("finalize payout execution: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDriverPayoutExecutionConflict
		}

		tag, err = tx.Exec(ctx, `
            UPDATE driver_payouts
            SET payout_status = 'PAID',
                provider = $2,
                provider_reference = $3,
                processed_at = clock_timestamp(),
                updated_at = clock_timestamp()
            WHERE id = $1
              AND payout_status = 'PENDING'
              AND provider IS NULL
              AND provider_reference IS NULL
              AND processed_at IS NULL
        `, parsedID, provider, providerReference)

		if err != nil {
			return fmt.Errorf("finalize driver payout: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return ErrDriverPayoutExecutionConflict
		}

		tag, err = tx.Exec(ctx, `
            UPDATE driver_earnings e
            SET settlement_status = 'PAID',
                settled_at = clock_timestamp(),
                updated_at = clock_timestamp()
            FROM driver_payout_items i
            WHERE i.earning_id = e.id
              AND i.payout_id = $1
              AND e.settlement_status = 'PROCESSING'
              AND e.settled_at IS NULL
        `, parsedID)

		if err != nil {
			return fmt.Errorf("finalize driver earnings: %w", err)
		}
		if tag.RowsAffected() != int64(itemCount) {
			return ErrDriverPayoutExecutionConflict
		}

		// The recovery lease may expire while financial rows
		// are being validated and updated. Recheck its lifetime
		// before allowing the transaction to commit.
		//
		// The lease row remains locked from the initial
		// SELECT FOR UPDATE until this transaction ends.
		if leaseToken != "" {
			var stillValid bool
			err := tx.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1
					FROM driver_payout_recovery_leases
					WHERE payout_id = $1::uuid
					  AND lease_token = $2::uuid
					  AND lease_expires_at >
					      clock_timestamp() +
					      ($3::bigint * interval '1 microsecond')
				)
			`, payoutID, leaseToken,
				(5 * time.Second).Microseconds(),
			).Scan(&stillValid)
			if err != nil {
				return fmt.Errorf(
					"revalidate recovery finalization lease: %w", err,
				)
			}
			if !stillValid {
				return ErrDriverPayoutRecoveryLeaseInvalid
			}
		}

		return nil
	})
}
