package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateDriverPayoutExecutionIntegration(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated settlement test database")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	defer cancel()

	db := openPayoutTestDatabase(t)

	var databaseName string

	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe database: %s", databaseName)
	}

	var driverID, companyID string

	err := db.QueryRow(ctx, `
        SELECT d.user_id, d.company_id
        FROM drivers d
        WHERE d.user_id IS NOT NULL
          AND d.company_id IS NOT NULL
        LIMIT 1
    `).Scan(&driverID, &companyID)

	if err != nil {
		t.Fatalf("driver fixture: %v", err)
	}

	reference := "connect-execution-create-" + uuid.NewString()

	var payoutID string

	err = db.QueryRow(ctx, `
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
        VALUES (
            $1, $2, $3,
            NOW() - INTERVAL '2 days',
            NOW() - INTERVAL '1 day',
            100.00, 7.00, 0.00,
            93.00, 'EUR', 'BANK_TRANSFER', 'PENDING'
        )
        RETURNING id
    `, driverID, companyID, reference).Scan(&payoutID)

	if err != nil {
		t.Fatalf("create synthetic payout: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			15*time.Second,
		)
		defer cleanupCancel()

		if _, err := db.Exec(cleanupCtx, `
            DELETE FROM driver_payout_executions
            WHERE payout_id = $1
        `, payoutID); err != nil {
			t.Errorf("cleanup payout execution: %v", err)
		}

		if _, err := db.Exec(cleanupCtx, `
            DELETE FROM driver_payouts
            WHERE id = $1
        `, payoutID); err != nil {
			t.Errorf("cleanup synthetic payout: %v", err)
		}
	})

	params := CreateDriverPayoutExecutionParams{
		PayoutID: payoutID,
		Provider: "TEST_PROVIDER",
	}

	t.Run("initial_creation", func(t *testing.T) {
		result, err := CreateDriverPayoutExecution(
			ctx, db, params,
		)

		if err != nil {
			t.Fatal(err)
		}

		if result == nil ||
			result.ID == "" ||
			result.PayoutID != payoutID ||
			result.Provider != "TEST_PROVIDER" ||
			result.Currency != "EUR" ||
			result.Status != "PENDING" ||
			result.AttemptCount != 0 {
			t.Fatalf("invalid execution: %+v", result)
		}

		expectedKey := "driver-payout-execution:" + payoutID

		if result.IdempotencyKey != expectedKey {
			t.Fatalf(
				"unexpected idempotency key: %s",
				result.IdempotencyKey,
			)
		}
	})

	t.Run("idempotent_replay", func(t *testing.T) {
		first, err := CreateDriverPayoutExecution(
			ctx, db, params,
		)
		if err != nil {
			t.Fatal(err)
		}

		replay, err := CreateDriverPayoutExecution(
			ctx, db, params,
		)
		if err != nil {
			t.Fatal(err)
		}

		if first.ID != replay.ID ||
			first.IdempotencyKey != replay.IdempotencyKey {
			t.Fatalf(
				"replay changed execution identity: %+v %+v",
				first, replay,
			)
		}
	})

	t.Run("provider_conflict", func(t *testing.T) {
		result, err := CreateDriverPayoutExecution(
			ctx,
			db,
			CreateDriverPayoutExecutionParams{
				PayoutID: payoutID,
				Provider: "OTHER_PROVIDER",
			},
		)

		if result != nil ||
			!errors.Is(
				err,
				ErrDriverPayoutExecutionConflict,
			) {
			t.Fatalf(
				"expected provider conflict: result=%+v err=%v",
				result, err,
			)
		}
	})

	t.Run("concurrent_replay", func(t *testing.T) {
		var wg sync.WaitGroup
		start := make(chan struct{})

		results := make([]*DriverPayoutExecution, 2)
		errs := make([]error, 2)

		for i := range results {
			wg.Add(1)

			go func(index int) {
				defer wg.Done()
				<-start

				results[index], errs[index] =
					CreateDriverPayoutExecution(
						ctx, db, params,
					)
			}(i)
		}

		close(start)
		wg.Wait()

		for i := range results {
			if errs[i] != nil {
				t.Fatalf(
					"concurrent execution %d: %v",
					i, errs[i],
				)
			}

			if results[i] == nil {
				t.Fatalf(
					"concurrent execution %d returned nil",
					i,
				)
			}
		}

		if results[0].ID != results[1].ID {
			t.Fatalf(
				"concurrent replay created different executions",
			)
		}
	})

	t.Run("concurrent_first_creation", func(t *testing.T) {
		reference := "connect-execution-race-" + uuid.NewString()

		var racePayoutID string

		err := db.QueryRow(ctx, `
            INSERT INTO driver_payouts (
                driver_id, company_id, payout_reference,
                period_start, period_end,
                gross_amount, deductions, adjustments,
                net_amount, currency, payout_method, payout_status
            )
            VALUES (
                $1, $2, $3,
                NOW() - INTERVAL '2 days',
                NOW() - INTERVAL '1 day',
                100.00, 7.00, 0.00,
                93.00, 'EUR', 'BANK_TRANSFER', 'PENDING'
            )
            RETURNING id
        `, driverID, companyID, reference).Scan(&racePayoutID)

		if err != nil {
			t.Fatalf("create race payout: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(
				context.Background(), 15*time.Second,
			)
			defer cancel()

			if _, err := db.Exec(cleanupCtx, `
                DELETE FROM driver_payout_executions
                WHERE payout_id = $1
            `, racePayoutID); err != nil {
				t.Errorf("cleanup race execution: %v", err)
			}

			if _, err := db.Exec(cleanupCtx, `
                DELETE FROM driver_payouts
                WHERE id = $1
            `, racePayoutID); err != nil {
				t.Errorf("cleanup race payout: %v", err)
			}
		})

		params := CreateDriverPayoutExecutionParams{
			PayoutID: racePayoutID,
			Provider: "TEST_PROVIDER",
		}

		var wg sync.WaitGroup
		start := make(chan struct{})

		results := make([]*DriverPayoutExecution, 2)
		errs := make([]error, 2)

		for i := range results {
			wg.Add(1)

			go func(index int) {
				defer wg.Done()
				<-start

				results[index], errs[index] =
					CreateDriverPayoutExecution(ctx, db, params)
			}(i)
		}

		close(start)
		wg.Wait()

		for i := range results {
			if errs[i] != nil {
				t.Fatalf("creation request %d: %v", i, errs[i])
			}

			if results[i] == nil {
				t.Fatalf("creation request %d returned nil", i)
			}
		}

		if results[0].ID != results[1].ID ||
			results[0].IdempotencyKey !=
				results[1].IdempotencyKey {
			t.Fatal("concurrent creation returned different identities")
		}

		var count int64
		var payoutStatus string
		var executionStatus string

		err = db.QueryRow(ctx, `
            SELECT
                (
                    SELECT COUNT(*)
                    FROM driver_payout_executions
                    WHERE payout_id = $1
                ),
                p.payout_status,
                e.execution_status
            FROM driver_payouts p
            JOIN driver_payout_executions e
              ON e.payout_id = p.id
            WHERE p.id = $1
        `, racePayoutID).Scan(
			&count,
			&payoutStatus,
			&executionStatus,
		)

		if err != nil {
			t.Fatal(err)
		}

		if count != 1 ||
			payoutStatus != "PENDING" ||
			executionStatus != "PENDING" {
			t.Fatalf(
				"unsafe concurrent creation: count=%d payout=%s execution=%s",
				count, payoutStatus, executionStatus,
			)
		}
	})

	t.Run("financial_state_unchanged", func(t *testing.T) {
		var executionCount int64
		var payoutStatus string
		var executionStatus string
		var attemptCount int

		err := db.QueryRow(ctx, `
            SELECT
                (
                    SELECT COUNT(*)
                    FROM driver_payout_executions
                    WHERE payout_id = $1
                ),
                p.payout_status,
                e.execution_status,
                e.attempt_count
            FROM driver_payouts p
            JOIN driver_payout_executions e
              ON e.payout_id = p.id
            WHERE p.id = $1
        `, payoutID).Scan(
			&executionCount,
			&payoutStatus,
			&executionStatus,
			&attemptCount,
		)

		if err != nil {
			t.Fatal(err)
		}

		if executionCount != 1 ||
			payoutStatus != "PENDING" ||
			executionStatus != "PENDING" ||
			attemptCount != 0 {
			t.Fatalf(
				"unsafe execution state: count=%d payout=%s execution=%s attempts=%d",
				executionCount,
				payoutStatus,
				executionStatus,
				attemptCount,
			)
		}
	})
}
