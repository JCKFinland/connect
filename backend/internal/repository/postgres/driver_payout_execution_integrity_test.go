package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDriverPayoutExecutionDatabaseIntegrity(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_settlement_test" ||
		os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated settlement test database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db := openPayoutTestDatabase(t)

	var databaseName string
	if err := db.QueryRow(ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe database: %s", databaseName)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	var driverID, companyID string

	err = tx.QueryRow(ctx, `
        SELECT d.user_id, d.company_id
        FROM drivers d
        WHERE d.user_id IS NOT NULL
          AND d.company_id IS NOT NULL
        LIMIT 1
    `).Scan(&driverID, &companyID)

	if err != nil {
		t.Fatalf("driver/company fixture: %v", err)
	}

	reference := "connect-execution-test-" + uuid.NewString()
	idempotencyKey := "connect-payout-execution:" + reference

	var payoutID string

	err = tx.QueryRow(ctx, `
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
		t.Fatalf("create synthetic payout: %v", err)
	}

	var executionID string

	err = tx.QueryRow(ctx, `
        INSERT INTO driver_payout_executions (
            payout_id, provider, idempotency_key,
            amount, currency
        )
        VALUES ($1, 'TEST_PROVIDER', $2, 93.00, 'EUR')
        RETURNING id
    `, payoutID, idempotencyKey).Scan(&executionID)

	if err != nil {
		t.Fatalf("create payout execution: %v", err)
	}

	expectRejected := func(name, sql string, args ...any) {
		t.Helper()

		savepoint := "SAVEPOINT payout_execution_probe"

		if _, err := tx.Exec(ctx, savepoint); err != nil {
			t.Fatalf("%s: create savepoint: %v", name, err)
		}

		_, operationErr := tx.Exec(ctx, sql, args...)

		if _, err := tx.Exec(ctx,
			"ROLLBACK TO SAVEPOINT payout_execution_probe",
		); err != nil {
			t.Fatalf("%s: rollback savepoint: %v", name, err)
		}

		if _, err := tx.Exec(ctx,
			"RELEASE SAVEPOINT payout_execution_probe",
		); err != nil {
			t.Fatalf("%s: release savepoint: %v", name, err)
		}

		if operationErr == nil {
			t.Fatalf("%s: invalid operation was accepted", name)
		}

		var pgErr *pgconn.PgError
		if !errors.As(operationErr, &pgErr) {
			t.Fatalf("%s: unexpected error: %v", name, operationErr)
		}
	}

	t.Run("duplicate_execution", func(t *testing.T) {
		expectRejected(
			"duplicate execution",
			`INSERT INTO driver_payout_executions (
                payout_id, provider, idempotency_key,
                amount, currency
            ) VALUES ($1, 'TEST_PROVIDER', $2, 93.00, 'EUR')`,
			payoutID,
			idempotencyKey+"-duplicate",
		)
	})

	t.Run("immutable_amount", func(t *testing.T) {
		expectRejected(
			"immutable amount",
			`UPDATE driver_payout_executions
             SET amount = 94.00 WHERE id = $1`,
			executionID,
		)
	})

	t.Run("immutable_idempotency", func(t *testing.T) {
		expectRejected(
			"immutable idempotency",
			`UPDATE driver_payout_executions
             SET idempotency_key = $2 WHERE id = $1`,
			executionID,
			idempotencyKey+"-changed",
		)
	})

	t.Run("immutable_parent", func(t *testing.T) {
		expectRejected(
			"immutable parent",
			`UPDATE driver_payouts
             SET net_amount = 94.00 WHERE id = $1`,
			payoutID,
		)
	})

	t.Run("premature_success", func(t *testing.T) {
		expectRejected(
			"premature success",
			`UPDATE driver_payout_executions
             SET execution_status = 'SUCCEEDED'
             WHERE id = $1`,
			executionID,
		)
	})

	if _, err := tx.Exec(ctx, `
        UPDATE driver_payout_executions
        SET execution_status = 'PROCESSING',
            request_started_at = NOW(),
            attempt_count = 1
        WHERE id = $1
    `, executionID); err != nil {
		t.Fatalf("start durable execution: %v", err)
	}

	t.Run("immutable_request_marker", func(t *testing.T) {
		expectRejected(
			"immutable request marker",
			`UPDATE driver_payout_executions
             SET request_started_at =
                 request_started_at + INTERVAL '1 second'
             WHERE id = $1`,
			executionID,
		)
	})

	if _, err := tx.Exec(ctx, `
        UPDATE driver_payout_executions
        SET execution_status = 'REQUIRES_RECONCILIATION'
        WHERE id = $1
    `, executionID); err != nil {
		t.Fatalf("enter reconciliation: %v", err)
	}

	t.Run("no_reconciliation_resubmit", func(t *testing.T) {
		expectRejected(
			"no reconciliation resubmit",
			`UPDATE driver_payout_executions
             SET execution_status = 'PROCESSING'
             WHERE id = $1`,
			executionID,
		)
	})

	t.Run("success_requires_confirmation", func(t *testing.T) {
		expectRejected(
			"success requires confirmation",
			`UPDATE driver_payout_executions
             SET execution_status = 'SUCCEEDED'
             WHERE id = $1`,
			executionID,
		)
	})

	t.Run("confirmation_requires_reference", func(t *testing.T) {
		expectRejected(
			"confirmation requires reference",
			`UPDATE driver_payout_executions
             SET provider_confirmed_at = NOW()
             WHERE id = $1`,
			executionID,
		)
	})

	// Synthetic provider identity; no external provider is contacted.
	providerReference := "synthetic-transfer-" + uuid.NewString()

	if _, err := tx.Exec(ctx, `
        UPDATE driver_payout_executions
        SET execution_status = 'SUCCEEDED',
            provider_reference = $2,
            provider_confirmed_at = NOW()
        WHERE id = $1
    `, executionID, providerReference); err != nil {
		t.Fatalf("record synthetic provider confirmation: %v", err)
	}

	t.Run("immutable_provider_reference", func(t *testing.T) {
		expectRejected(
			"immutable provider reference",
			`UPDATE driver_payout_executions
             SET provider_reference = $2
             WHERE id = $1`,
			executionID,
			"modified-"+providerReference,
		)
	})

	t.Run("immutable_confirmation_timestamp", func(t *testing.T) {
		expectRejected(
			"immutable confirmation timestamp",
			`UPDATE driver_payout_executions
             SET provider_confirmed_at =
                 provider_confirmed_at + INTERVAL '1 second'
             WHERE id = $1`,
			executionID,
		)
	})

	t.Run("terminal_success_cannot_reopen", func(t *testing.T) {
		expectRejected(
			"terminal success cannot reopen",
			`UPDATE driver_payout_executions
             SET execution_status = 'PROCESSING'
             WHERE id = $1`,
			executionID,
		)
	})

	var status string
	err = tx.QueryRow(ctx, `
        SELECT execution_status
        FROM driver_payout_executions
        WHERE id = $1
    `, executionID).Scan(&status)

	if err != nil {
		t.Fatal(err)
	}

	if status != "SUCCEEDED" {
		t.Fatalf("unexpected final execution status: %s", status)
	}

	// The entire fixture and all transitions are rolled back.
	if err := tx.Rollback(ctx); err != nil &&
		!errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("rollback payout fixture: %v", err)
	}
}
