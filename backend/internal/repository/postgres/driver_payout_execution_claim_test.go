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

func TestClaimDriverPayoutExecutionIntegration(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated settlement test database")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 60*time.Second,
	)
	defer cancel()

	db := openPayoutTestDatabase(t)

	var databaseName string
	if err := db.QueryRow(
		ctx, "SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe database: %s", databaseName)
	}

	var driverID, companyID string

	if err := db.QueryRow(ctx, `
        SELECT d.user_id, d.company_id
        FROM drivers d
        WHERE d.user_id IS NOT NULL
          AND d.company_id IS NOT NULL
        LIMIT 1
    `).Scan(&driverID, &companyID); err != nil {
		t.Fatalf("driver fixture: %v", err)
	}

	createPayout := func(t *testing.T) string {
		t.Helper()

		reference := "connect-execution-claim-" + uuid.NewString()

		var payoutID string

		err := db.QueryRow(ctx, `
            INSERT INTO driver_payouts (
                driver_id, company_id, payout_reference,
                period_start, period_end,
                gross_amount, deductions, adjustments,
                net_amount, currency,
                payout_method, payout_status
            )
            VALUES (
                $1, $2, $3,
                NOW() - INTERVAL '2 days',
                NOW() - INTERVAL '1 day',
                100.00, 7.00, 0.00,
                93.00, 'EUR',
                'BANK_TRANSFER', 'PENDING'
            )
            RETURNING id
        `, driverID, companyID, reference).Scan(&payoutID)

		if err != nil {
			t.Fatalf("create synthetic payout: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(
				context.Background(), 15*time.Second,
			)
			defer cleanupCancel()

			if _, err := db.Exec(cleanupCtx, `
                DELETE FROM driver_payout_executions
                WHERE payout_id = $1
            `, payoutID); err != nil {
				t.Errorf("cleanup execution: %v", err)
			}

			if _, err := db.Exec(cleanupCtx, `
                DELETE FROM driver_payouts
                WHERE id = $1
            `, payoutID); err != nil {
				t.Errorf("cleanup payout: %v", err)
			}
		})

		return payoutID
	}

	createExecution := func(t *testing.T, payoutID string) {
		t.Helper()

		result, err := CreateDriverPayoutExecution(
			ctx,
			db,
			CreateDriverPayoutExecutionParams{
				PayoutID: payoutID,
				Provider: "TEST_PROVIDER",
			},
		)

		if err != nil || result == nil ||
			result.Status != "PENDING" {
			t.Fatalf(
				"create execution: result=%+v err=%v",
				result, err,
			)
		}
	}

	t.Run("first_claim_and_replay", func(t *testing.T) {
		payoutID := createPayout(t)
		createExecution(t, payoutID)

		first, claimed, err := ClaimDriverPayoutExecution(
			ctx, db, payoutID,
		)

		if err != nil || !claimed || first == nil {
			t.Fatalf(
				"first claim: result=%+v claimed=%v err=%v",
				first, claimed, err,
			)
		}

		if first.Status != "PROCESSING" ||
			first.AttemptCount != 1 {
			t.Fatalf("invalid first claim: %+v", first)
		}

		replay, claimed, err := ClaimDriverPayoutExecution(
			ctx, db, payoutID,
		)

		if err != nil || claimed || replay == nil {
			t.Fatalf(
				"replay: result=%+v claimed=%v err=%v",
				replay, claimed, err,
			)
		}

		if replay.ID != first.ID ||
			replay.Status != "PROCESSING" ||
			replay.AttemptCount != 1 {
			t.Fatalf("unsafe replay: %+v", replay)
		}

		var requestStarted bool
		var attemptCount int
		var status string

		err = db.QueryRow(ctx, `
            SELECT
                request_started_at IS NOT NULL,
                attempt_count,
                execution_status
            FROM driver_payout_executions
            WHERE payout_id = $1
        `, payoutID).Scan(
			&requestStarted, &attemptCount, &status,
		)

		if err != nil {
			t.Fatal(err)
		}

		if !requestStarted ||
			attemptCount != 1 ||
			status != "PROCESSING" {
			t.Fatalf(
				"unsafe persisted claim: started=%v attempts=%d status=%s",
				requestStarted, attemptCount, status,
			)
		}
	})

	t.Run("concurrent_first_claim", func(t *testing.T) {
		payoutID := createPayout(t)
		createExecution(t, payoutID)

		const callers = 8

		results := make([]*DriverPayoutExecution, callers)
		claimed := make([]bool, callers)
		errs := make([]error, callers)

		var wg sync.WaitGroup
		start := make(chan struct{})

		for i := 0; i < callers; i++ {
			wg.Add(1)

			go func(index int) {
				defer wg.Done()
				<-start

				results[index], claimed[index], errs[index] =
					ClaimDriverPayoutExecution(
						ctx, db, payoutID,
					)
			}(i)
		}

		close(start)
		wg.Wait()

		winners := 0
		executionID := ""

		for i := 0; i < callers; i++ {
			if errs[i] != nil || results[i] == nil {
				t.Fatalf(
					"caller %d: result=%+v err=%v",
					i, results[i], errs[i],
				)
			}

			if claimed[i] {
				winners++
			}

			if executionID == "" {
				executionID = results[i].ID
			}

			if results[i].ID != executionID ||
				results[i].Status != "PROCESSING" ||
				results[i].AttemptCount != 1 {
				t.Fatalf(
					"unsafe concurrent result %d: %+v",
					i, results[i],
				)
			}
		}

		if winners != 1 {
			t.Fatalf(
				"expected exactly one winner, got %d",
				winners,
			)
		}

		var count int
		var attempts int
		var requestStarted bool

		err := db.QueryRow(ctx, `
            SELECT
                COUNT(*),
                MAX(attempt_count),
                BOOL_AND(request_started_at IS NOT NULL)
            FROM driver_payout_executions
            WHERE payout_id = $1
        `, payoutID).Scan(
			&count, &attempts, &requestStarted,
		)

		if err != nil {
			t.Fatal(err)
		}

		if count != 1 || attempts != 1 || !requestStarted {
			t.Fatalf(
				"unsafe persisted concurrency: count=%d attempts=%d started=%v",
				count, attempts, requestStarted,
			)
		}
	})

	t.Run("missing_execution", func(t *testing.T) {
		payoutID := createPayout(t)

		result, claimed, err := ClaimDriverPayoutExecution(
			ctx, db, payoutID,
		)

		if result != nil || claimed ||
			!errors.Is(err, ErrDriverPayoutExecutionConflict) {
			t.Fatalf(
				"expected missing execution conflict: result=%+v claimed=%v err=%v",
				result, claimed, err,
			)
		}
	})

	t.Run("invalid_input", func(t *testing.T) {
		result, claimed, err := ClaimDriverPayoutExecution(
			ctx, db, "invalid-uuid",
		)

		if result != nil || claimed ||
			!errors.Is(err, ErrDriverPayoutExecutionInvalidInput) {
			t.Fatalf(
				"expected invalid input: result=%+v claimed=%v err=%v",
				result, claimed, err,
			)
		}

		result, claimed, err = ClaimDriverPayoutExecution(
			ctx, nil, uuid.NewString(),
		)

		if result != nil || claimed ||
			!errors.Is(err, ErrDriverPayoutExecutionInvalidInput) {
			t.Fatalf(
				"expected nil database rejection: result=%+v claimed=%v err=%v",
				result, claimed, err,
			)
		}
	})

	t.Run("reconciliation_cannot_reclaim", func(t *testing.T) {
		payoutID := createPayout(t)
		createExecution(t, payoutID)

		_, claimed, err := ClaimDriverPayoutExecution(
			ctx, db, payoutID,
		)

		if err != nil || !claimed {
			t.Fatalf("initial claim: claimed=%v err=%v", claimed, err)
		}

		_, err = db.Exec(ctx, `
            UPDATE driver_payout_executions
            SET
                execution_status = 'REQUIRES_RECONCILIATION',
                updated_at = NOW()
            WHERE payout_id = $1
        `, payoutID)

		if err != nil {
			t.Fatal(err)
		}

		result, claimed, err := ClaimDriverPayoutExecution(
			ctx, db, payoutID,
		)

		if err != nil || claimed || result == nil ||
			result.Status != "REQUIRES_RECONCILIATION" ||
			result.AttemptCount != 1 {
			t.Fatalf(
				"unsafe reconciliation replay: result=%+v claimed=%v err=%v",
				result, claimed, err,
			)
		}
	})
}
