package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDriverPayoutUnknownOutcomeReconciliation(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated settlement test database")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 90*time.Second,
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

	createPayout := func(t *testing.T) string {
		t.Helper()

		var driverID, companyID string

		err := db.QueryRow(ctx, `
            SELECT d.user_id, d.company_id
            FROM drivers d
            WHERE d.user_id IS NOT NULL
              AND d.company_id IS NOT NULL
            LIMIT 1
        `).Scan(&driverID, &companyID)
		if err != nil {
			t.Fatal(err)
		}

		reference := "connect-reconcile-" + uuid.NewString()

		var payoutID string

		err = db.QueryRow(ctx, `
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
        `, driverID, companyID, reference).Scan(&payoutID)

		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()

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

		_, err := CreateDriverPayoutExecution(
			ctx, db,
			CreateDriverPayoutExecutionParams{
				PayoutID: payoutID,
				Provider: "TEST_PROVIDER",
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	claim := func(t *testing.T, payoutID string) {
		t.Helper()

		_, claimed, err := ClaimDriverPayoutExecution(
			ctx, db, payoutID,
		)
		if err != nil || !claimed {
			t.Fatalf("claim: claimed=%v err=%v", claimed, err)
		}
	}

	assertFinancialState := func(t *testing.T, payoutID string) {
		t.Helper()

		var status, gross, net string
		var providerReference *string

		err := db.QueryRow(ctx, `
            SELECT payout_status,
                   gross_amount::text,
                   net_amount::text,
                   provider_reference
            FROM driver_payouts
            WHERE id = $1
        `, payoutID).Scan(
			&status, &gross, &net, &providerReference,
		)
		if err != nil {
			t.Fatal(err)
		}

		if status != "PENDING" ||
			gross != "100.00" ||
			net != "93.00" ||
			providerReference != nil {
			t.Fatalf(
				"financial state changed: status=%s gross=%s net=%s ref=%v",
				status, gross, net, providerReference,
			)
		}
	}

	t.Run("first_reconciliation_and_replay", func(t *testing.T) {
		payoutID := createPayout(t)
		createExecution(t, payoutID)
		claim(t, payoutID)

		first, err := MarkDriverPayoutRequiresReconciliation(
			ctx, db, payoutID, "provider timeout",
		)
		if err != nil || first == nil ||
			first.Status != "REQUIRES_RECONCILIATION" ||
			first.AttemptCount != 1 {
			t.Fatalf("first reconciliation: %+v err=%v", first, err)
		}

		replay, err := MarkDriverPayoutRequiresReconciliation(
			ctx, db, payoutID, "provider timeout",
		)
		if err != nil || replay == nil ||
			replay.ID != first.ID ||
			replay.Status != "REQUIRES_RECONCILIATION" {
			t.Fatalf("replay: %+v err=%v", replay, err)
		}

		var attempts int
		var reason string
		var requestStarted *time.Time

		err = db.QueryRow(ctx, `
            SELECT attempt_count, last_error, request_started_at
            FROM driver_payout_executions
            WHERE payout_id = $1
        `, payoutID).Scan(&attempts, &reason, &requestStarted)

		if err != nil || attempts != 1 ||
			reason != "provider timeout" ||
			requestStarted == nil {
			t.Fatalf(
				"invalid durable state: attempts=%d reason=%q started=%v err=%v",
				attempts, reason, requestStarted, err,
			)
		}

		assertFinancialState(t, payoutID)
	})

	t.Run("premature_reconciliation_rejected", func(t *testing.T) {
		payoutID := createPayout(t)
		createExecution(t, payoutID)

		result, err := MarkDriverPayoutRequiresReconciliation(
			ctx, db, payoutID, "not submitted",
		)
		if result != nil ||
			!errors.Is(err, ErrDriverPayoutExecutionConflict) {
			t.Fatalf("expected conflict: %+v %v", result, err)
		}

		assertFinancialState(t, payoutID)
	})

	t.Run("missing_execution_rejected", func(t *testing.T) {
		payoutID := createPayout(t)

		result, err := MarkDriverPayoutRequiresReconciliation(
			ctx, db, payoutID, "unknown",
		)
		if result != nil ||
			!errors.Is(err, ErrDriverPayoutExecutionConflict) {
			t.Fatalf("expected conflict: %+v %v", result, err)
		}

		assertFinancialState(t, payoutID)
	})

	t.Run("invalid_input_rejected", func(t *testing.T) {
		cases := []struct {
			db     *pgxpool.Pool
			payout string
			reason string
		}{
			{db, "invalid", "timeout"},
			{nil, uuid.NewString(), "timeout"},
			{db, uuid.NewString(), ""},
			{db, uuid.NewString(), "   "},
		}

		for _, tc := range cases {
			result, err := MarkDriverPayoutRequiresReconciliation(
				ctx, tc.db, tc.payout, tc.reason,
			)
			if result != nil ||
				!errors.Is(err, ErrDriverPayoutExecutionInvalidInput) {
				t.Fatalf("invalid input accepted: %+v %v", result, err)
			}
		}
	})

	t.Run("terminal_success_rejected", func(t *testing.T) {
		payoutID := createPayout(t)
		createExecution(t, payoutID)
		claim(t, payoutID)

		_, err := db.Exec(ctx, `
            UPDATE driver_payout_executions
            SET execution_status = 'SUCCEEDED',
                provider_reference = $2,
                provider_confirmed_at = NOW()
            WHERE payout_id = $1
        `, payoutID, "synthetic-"+uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}

		result, err := MarkDriverPayoutRequiresReconciliation(
			ctx, db, payoutID, "late timeout",
		)
		if result != nil ||
			!errors.Is(err, ErrDriverPayoutExecutionConflict) {
			t.Fatalf("terminal state changed: %+v %v", result, err)
		}

		assertFinancialState(t, payoutID)
	})

	t.Run("concurrent_reconciliation", func(t *testing.T) {
		payoutID := createPayout(t)
		createExecution(t, payoutID)
		claim(t, payoutID)

		const workers = 8

		var wg sync.WaitGroup
		results := make(chan error, workers)

		for i := 0; i < workers; i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				result, err := MarkDriverPayoutRequiresReconciliation(
					ctx, db, payoutID, "provider outcome unknown",
				)

				if err == nil &&
					(result == nil ||
						result.Status != "REQUIRES_RECONCILIATION") {
					err = errors.New("incorrect reconciliation result")
				}

				results <- err
			}()
		}

		wg.Wait()
		close(results)

		for err := range results {
			if err != nil {
				t.Fatal(err)
			}
		}

		var count, attempts int

		err := db.QueryRow(ctx, `
            SELECT COUNT(*), COALESCE(MAX(attempt_count), 0)
            FROM driver_payout_executions
            WHERE payout_id = $1
        `, payoutID).Scan(&count, &attempts)

		if err != nil || count != 1 || attempts != 1 {
			t.Fatalf(
				"duplicate execution: count=%d attempts=%d err=%v",
				count, attempts, err,
			)
		}

		assertFinancialState(t, payoutID)
	})
}
