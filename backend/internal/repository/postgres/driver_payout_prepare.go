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

var (
	ErrDriverPayoutInvalidInput = errors.New(
		"invalid driver payout preparation input",
	)
	ErrDriverPayoutConflict = errors.New(
		"driver payout reference conflict",
	)
	ErrDriverPayoutNoEligibleEarnings = errors.New(
		"no eligible FLEX driver earnings",
	)
)

type PreparedDriverPayout struct {
	ID              string
	DriverID        string
	CompanyID       string
	PayoutReference string
	PeriodStart     time.Time
	PeriodEnd       time.Time
	GrossAmount     string
	NetAmount       string
	Currency        string
	PayoutMethod    string
	PayoutStatus    string
	ItemCount       int64
}

type PrepareDriverPayoutParams struct {
	DriverID        string
	CompanyID       string
	PayoutReference string
	PeriodStart     time.Time
	PeriodEnd       time.Time
}

func PrepareFlexDriverPayout(
	ctx context.Context,
	db *pgxpool.Pool,
	params PrepareDriverPayoutParams,
) (*PreparedDriverPayout, error) {
	if db == nil {
		return nil, ErrDriverPayoutInvalidInput
	}

	driverID, err := uuid.Parse(params.DriverID)
	if err != nil {
		return nil, ErrDriverPayoutInvalidInput
	}

	companyID, err := uuid.Parse(params.CompanyID)
	if err != nil {
		return nil, ErrDriverPayoutInvalidInput
	}

	// Canonical identities ensure equivalent UUID representations
	// acquire identical PostgreSQL advisory locks.
	params.DriverID = driverID.String()
	params.CompanyID = companyID.String()

	// PostgreSQL stores timestamps at microsecond precision.
	// Canonicalize both payout period boundaries before persisting
	// or comparing them during idempotent replay.
	params.PeriodStart = params.PeriodStart.UTC().Truncate(time.Microsecond)
	params.PeriodEnd = params.PeriodEnd.UTC().Truncate(time.Microsecond)

	params.PayoutReference = strings.TrimSpace(
		params.PayoutReference,
	)

	if params.PayoutReference == "" ||
		len(params.PayoutReference) > 100 ||
		params.PeriodStart.IsZero() ||
		params.PeriodEnd.IsZero() ||
		!params.PeriodStart.Before(params.PeriodEnd) {
		return nil, ErrDriverPayoutInvalidInput
	}

	var result *PreparedDriverPayout

	err = RunInTransaction(ctx, db, func(tx pgx.Tx) error {
		// Lock the globally unique payout reference first.
		// Requests using the same reference must serialize,
		// even when their driver/company identities differ.
		referenceLockKey := "driver-payout-reference:" +
			params.PayoutReference

		if err := AcquireTransactionAdvisoryLock(
			ctx,
			tx,
			referenceLockKey,
		); err != nil {
			return err
		}

		// Serialize earning claims for the same driver/company.
		// All callers acquire locks in the same order.
		ownerLockKey := "driver-payout-owner:" +
			params.DriverID + ":" + params.CompanyID

		if err := AcquireTransactionAdvisoryLock(
			ctx,
			tx,
			ownerLockKey,
		); err != nil {
			return err
		}

		existing, err := readPreparedDriverPayout(
			ctx,
			tx,
			params.PayoutReference,
		)

		if err == nil {
			if existing.DriverID != params.DriverID ||
				existing.CompanyID != params.CompanyID ||
				!existing.PeriodStart.Equal(params.PeriodStart) ||
				!existing.PeriodEnd.Equal(params.PeriodEnd) ||
				existing.Currency != "EUR" ||
				existing.PayoutMethod != "BANK_TRANSFER" {
				return ErrDriverPayoutConflict
			}

			result = existing
			return nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf(
				"read existing driver payout: %w",
				err,
			)
		}
		// Acquire payment-scoped locks before earning row locks.
		// Sorting guarantees a consistent order for multi-payment payouts.
		paymentRows, err := tx.Query(ctx, `
                SELECT DISTINCT e.payment_id::text
                FROM driver_earnings e
                WHERE e.driver_id = $1
                  AND e.company_id = $2
                  AND e.payment_id IS NOT NULL
                  AND e.calculated_at >= $3
                  AND e.calculated_at < $4
                ORDER BY e.payment_id::text
            `,
			params.DriverID,
			params.CompanyID,
			params.PeriodStart,
			params.PeriodEnd,
		)
		if err != nil {
			return fmt.Errorf("list payout payment locks: %w", err)
		}

		paymentIDs := make([]string, 0)
		for paymentRows.Next() {
			var paymentID string
			if err := paymentRows.Scan(&paymentID); err != nil {
				paymentRows.Close()
				return err
			}
			paymentIDs = append(paymentIDs, paymentID)
		}
		if err := paymentRows.Err(); err != nil {
			paymentRows.Close()
			return err
		}
		paymentRows.Close()

		for _, paymentID := range paymentIDs {
			if err := AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"payment-financial-boundary:"+paymentID,
			); err != nil {
				return fmt.Errorf("lock payout financial boundary: %w", err)
			}
		}

		earnings := make([]payoutEligibleEarning, 0)

		rows, err := tx.Query(ctx, `
			SELECT
				e.id,
				e.gross_amount::text,
				e.net_amount::text
			FROM driver_earnings e
			JOIN trips t
				ON t.id = e.trip_id
			JOIN payments p
				ON p.id = e.payment_id
			JOIN flex_settlements fs
				ON fs.trip_id = e.trip_id
				AND fs.payment_id = p.id
			JOIN ride_requests rr
				ON rr.id = t.ride_request_id
			WHERE e.driver_id = $1
			  AND e.company_id = $2
			  AND t.driver_id = e.driver_id
			  AND t.company_id = e.company_id
			  AND t.status = 'COMPLETED'
			  AND t.deleted_at IS NULL
			  AND rr.matching_mode = 'FLEX'
                    AND p.status = 'PAID'
                    AND NOT EXISTS (
                        SELECT 1
                        FROM payment_transactions refund_tx
                        WHERE refund_tx.payment_id = p.id
                          AND refund_tx.transaction_type = 'REFUND'
                          AND refund_tx.status IN (
                              'PENDING',
                              'PROCESSING',
                              'SUCCESS'
                          )
                    )
			  AND p.trip_id = t.id
			  AND p.currency = e.currency
			  AND p.amount = e.gross_amount
			  AND e.commission_amount = ROUND(e.gross_amount * 0.07, 2)
			  AND e.net_amount = e.gross_amount - e.commission_amount
			  AND e.bonus_amount = 0
			  AND e.tip_amount = 0
			  AND e.adjustment_amount = 0
			  AND e.tax_withheld = 0
			  AND fs.status = 'CAPTURED'
			  AND fs.currency = e.currency
			  AND fs.amount = p.amount
			  AND fs.provider = 'STRIPE'
			  AND fs.provider_capture_id IS NOT NULL
			  AND e.currency = 'EUR'
			  AND e.settlement_status IN ('PENDING', 'READY')
			  AND e.calculated_at >= $3
			  AND e.calculated_at < $4
			  AND NOT EXISTS (
				  SELECT 1
				  FROM driver_payout_items dpi
				  WHERE dpi.earning_id = e.id
			  )
			ORDER BY e.id
			FOR UPDATE OF e
		`,
			params.DriverID,
			params.CompanyID,
			params.PeriodStart,
			params.PeriodEnd,
		)

		if err != nil {
			return fmt.Errorf(
				"select eligible FLEX earnings: %w",
				err,
			)
		}

		for rows.Next() {
			var item payoutEligibleEarning

			if err := rows.Scan(
				&item.id,
				&item.gross,
				&item.net,
			); err != nil {
				rows.Close()
				return err
			}

			earnings = append(earnings, item)
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}

		rows.Close()

		if len(earnings) == 0 {
			return ErrDriverPayoutNoEligibleEarnings
		}

		var payoutID string

		err = tx.QueryRow(ctx, `
			INSERT INTO driver_payouts (
				driver_id,
				company_id,
				payout_reference,
				period_start,
				period_end,
				gross_amount,
				deductions,
				adjustments,
				net_amount,
				currency,
				payout_method,
				payout_status
			)
			SELECT
				$1,
				$2,
				$3,
				$4,
				$5,
				SUM(e.gross_amount),
				SUM(e.gross_amount - e.net_amount),
				0,
				SUM(e.net_amount),
				'EUR',
				'BANK_TRANSFER',
				'PENDING'
			FROM driver_earnings e
			WHERE e.id = ANY($6::uuid[])
			HAVING COUNT(*) = $7
			RETURNING id
		`,
			params.DriverID,
			params.CompanyID,
			params.PayoutReference,
			params.PeriodStart,
			params.PeriodEnd,
			earningIDs(earnings),
			len(earnings),
		).Scan(&payoutID)

		if err != nil {
			return fmt.Errorf(
				"create driver payout: %w",
				err,
			)
		}

		for _, item := range earnings {
			_, err := tx.Exec(ctx, `
				INSERT INTO driver_payout_items (
					payout_id,
					earning_id,
					amount
				)
				VALUES ($1, $2, $3::numeric)
			`,
				payoutID,
				item.id,
				item.net,
			)

			if err != nil {
				return fmt.Errorf(
					"claim driver earning: %w",
					err,
				)
			}
		}

		tag, err := tx.Exec(ctx, `
			UPDATE driver_earnings
			SET settlement_status = 'PROCESSING',
			    updated_at = clock_timestamp()
			WHERE id = ANY($1::uuid[])
			  AND settlement_status IN ('PENDING', 'READY')
		`, earningIDs(earnings))

		if err != nil {
			return fmt.Errorf(
				"update claimed earning status: %w",
				err,
			)
		}

		if tag.RowsAffected() != int64(len(earnings)) {
			return ErrDriverPayoutConflict
		}

		result, err = readPreparedDriverPayoutByID(
			ctx,
			tx,
			payoutID,
		)

		return err
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

type payoutEligibleEarning struct {
	id    string
	gross string
	net   string
}

func earningIDs(items []payoutEligibleEarning) []string {
	ids := make([]string, 0, len(items))

	for _, item := range items {
		ids = append(ids, item.id)
	}

	return ids
}

func readPreparedDriverPayout(
	ctx context.Context,
	tx pgx.Tx,
	reference string,
) (*PreparedDriverPayout, error) {
	return scanPreparedDriverPayout(
		tx.QueryRow(ctx, `
			SELECT
				dp.id,
				dp.driver_id,
				dp.company_id,
				dp.payout_reference,
				dp.period_start,
				dp.period_end,
				dp.gross_amount::text,
				dp.net_amount::text,
				dp.currency,
				dp.payout_method,
				dp.payout_status,
				COUNT(dpi.id)
			FROM driver_payouts dp
			LEFT JOIN driver_payout_items dpi
				ON dpi.payout_id = dp.id
			WHERE dp.payout_reference = $1
			GROUP BY dp.id
		`, reference),
	)
}

func readPreparedDriverPayoutByID(
	ctx context.Context,
	tx pgx.Tx,
	id string,
) (*PreparedDriverPayout, error) {
	return scanPreparedDriverPayout(
		tx.QueryRow(ctx, `
			SELECT
				dp.id,
				dp.driver_id,
				dp.company_id,
				dp.payout_reference,
				dp.period_start,
				dp.period_end,
				dp.gross_amount::text,
				dp.net_amount::text,
				dp.currency,
				dp.payout_method,
				dp.payout_status,
				COUNT(dpi.id)
			FROM driver_payouts dp
			LEFT JOIN driver_payout_items dpi
				ON dpi.payout_id = dp.id
			WHERE dp.id = $1
			GROUP BY dp.id
		`, id),
	)
}

type preparedPayoutScanner interface {
	Scan(dest ...any) error
}

func scanPreparedDriverPayout(
	row preparedPayoutScanner,
) (*PreparedDriverPayout, error) {
	result := &PreparedDriverPayout{}

	err := row.Scan(
		&result.ID,
		&result.DriverID,
		&result.CompanyID,
		&result.PayoutReference,
		&result.PeriodStart,
		&result.PeriodEnd,
		&result.GrossAmount,
		&result.NetAmount,
		&result.Currency,
		&result.PayoutMethod,
		&result.PayoutStatus,
		&result.ItemCount,
	)

	if err != nil {
		return nil, err
	}

	return result, nil
}
