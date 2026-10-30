package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecoveryTransferRequestRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name     string
		ctx      context.Context
		payoutID string
	}{
		{
			name:     "nil_context",
			ctx:      nil,
			payoutID: "invalid",
		},
		{
			name:     "invalid_payout_id",
			ctx:      context.Background(),
			payoutID: "invalid",
		},
		{
			name:     "empty_payout_id",
			ctx:      context.Background(),
			payoutID: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ResolveDriverPayoutRecoveryTransferRequest(
				tc.ctx,
				nil,
				tc.payoutID,
			)

			if result != nil ||
				!errors.Is(err, ErrDriverPayoutExecutionInvalidInput) {
				t.Fatalf(
					"invalid input accepted: result=%+v err=%v",
					result,
					err,
				)
			}
		})
	}
}

func TestRecoveryTransferRequestIntegration(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" {
		t.Skip("requires isolated payout destination database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must be disabled")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 90*time.Second,
	)
	defer cancel()

	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}

	cfg, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%s user=%s dbname=connect_payout_destination_test sslmode=disable",
		host, port, user,
	))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 4

	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	if err := db.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	var databaseName string
	var migrationVersion int

	err = db.QueryRow(ctx, `
        SELECT current_database(),
            (SELECT MAX(version_id)
             FROM goose_db_version
             WHERE is_applied=true)
    `).Scan(&databaseName, &migrationVersion)

	if err != nil {
		t.Fatal(err)
	}
	if databaseName != "connect_payout_destination_test" ||
		(migrationVersion != 74 && migrationVersion != 75) {
		t.Fatalf(
			"unsafe database %s migration %d",
			databaseName, migrationVersion,
		)
	}

	driverID := uuid.NewString()
	companyID := uuid.NewString()
	suffix := uuid.NewString()

	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer stop()

		statements := []struct {
			query string
			id    string
		}{
			{`DELETE FROM driver_payout_executions
              WHERE payout_id IN (
                SELECT id FROM driver_payouts WHERE driver_id=$1
              )`, driverID},
			{`DELETE FROM driver_payouts
              WHERE driver_id=$1`, driverID},
			{`DELETE FROM driver_payout_destinations
              WHERE driver_id=$1`, driverID},
			{`DELETE FROM companies WHERE id=$1`, companyID},
			{`DELETE FROM users WHERE id=$1`, driverID},
		}

		for _, statement := range statements {
			if _, err := db.Exec(
				cleanupCtx, statement.query, statement.id,
			); err != nil {
				t.Errorf("fixture cleanup: %v", err)
			}
		}
	})

	_, err = db.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash, first_name, last_name
        )
        VALUES ($1, $2, $3, 'Recovery', 'Fixture')
    `, driverID, "recovery-"+suffix+"@example.invalid",
		"integration-test-only")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, country_code, timezone
        )
        VALUES ($1, $2, $3, 'FI', 'Europe/Helsinki')
    `, companyID, "Recovery "+suffix, "Recovery "+suffix)
	if err != nil {
		t.Fatal(err)
	}

	var payoutID string
	err = db.QueryRow(ctx, `
        INSERT INTO driver_payouts (
            driver_id, company_id, payout_reference,
            period_start, period_end,
            gross_amount, deductions, adjustments,
            net_amount, currency,
            payout_method, payout_status
        )
        VALUES (
            $1, $2, $3,
            NOW()-INTERVAL '2 days',
            NOW()-INTERVAL '1 day',
            25.00, 3.35, 0.00, 21.65,
            'EUR', 'BANK_TRANSFER', 'PENDING'
        )
        RETURNING id
    `, driverID, companyID,
		"recovery-"+uuid.NewString(),
	).Scan(&payoutID)
	if err != nil {
		t.Fatal(err)
	}

	externalID := "recovery-destination-" + uuid.NewString()
	var destinationID string

	err = db.QueryRow(ctx, `
        INSERT INTO driver_payout_destinations (
            driver_id, company_id, provider,
            provider_destination_id
        )
        VALUES ($1, $2, 'TEST_PROVIDER', $3)
        RETURNING id
    `, driverID, companyID, externalID).Scan(&destinationID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx, `
        UPDATE driver_payout_destinations
        SET verification_status='VERIFIED',
            verified_at=NOW()
        WHERE id=$1
    `, destinationID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = CreateBoundDriverPayoutExecution(
		ctx, db,
		CreateBoundDriverPayoutExecutionParams{
			PayoutID:      payoutID,
			Provider:      "TEST_PROVIDER",
			DestinationID: destinationID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolve := func() (*payouts.TransferRequest, error) {
		t.Helper()
		return ResolveDriverPayoutRecoveryTransferRequest(
			ctx, db, payoutID,
		)
	}

	// Unclaimed execution must be rejected.
	result, err := resolve()
	if result != nil ||
		!errors.Is(err, ErrDriverPayoutExecutionConflict) {
		t.Fatalf("unclaimed payout accepted: %+v %v", result, err)
	}

	claimed, didClaim, err := ClaimDriverPayoutExecution(
		ctx, db, payoutID,
	)
	if err != nil || !didClaim || claimed == nil {
		t.Fatalf(
			"claim failed: %+v claimed=%v err=%v",
			claimed, didClaim, err,
		)
	}

	// PROCESSING must resolve the exact immutable identity.
	processing, err := resolve()
	if err != nil {
		t.Fatal(err)
	}

	if processing.PayoutID != payoutID ||
		processing.IdempotencyKey != claimed.IdempotencyKey ||
		processing.DestinationID != externalID ||
		processing.AmountCents != 2165 ||
		processing.Currency != "EUR" {
		t.Fatalf("incorrect PROCESSING identity: %+v", processing)
	}

	// Transition using the existing repository operation.
	_, err = MarkDriverPayoutRequiresReconciliation(
		ctx, db, payoutID, "integration verification",
	)
	if err != nil {
		t.Fatal(err)
	}

	// REQUIRES_RECONCILIATION must retain the same identity.
	reconciliation, err := resolve()
	if err != nil {
		t.Fatal(err)
	}

	if *reconciliation != *processing {
		t.Fatalf(
			"identity changed across recovery state: %+v %+v",
			processing, reconciliation,
		)
	}

	// Resolver must never create another submission claim.
	_, didClaim, err = ClaimDriverPayoutExecution(
		ctx, db, payoutID,
	)
	if err != nil || didClaim {
		t.Fatalf(
			"second claim authorized: claimed=%v err=%v",
			didClaim, err,
		)
	}

	var attempts int
	var status string
	err = db.QueryRow(ctx, `
        SELECT attempt_count, execution_status
        FROM driver_payout_executions
        WHERE payout_id=$1
    `, payoutID).Scan(&attempts, &status)
	if err != nil {
		t.Fatal(err)
	}

	if attempts != 1 || status != "REQUIRES_RECONCILIATION" {
		t.Fatalf(
			"resolver mutated execution: attempts=%d status=%s",
			attempts, status,
		)
	}

	// A terminal payout cannot authorize recovery verification.
	_, err = db.Exec(ctx, `
        UPDATE driver_payouts
        SET payout_status='PAID'
        WHERE id=$1
    `, payoutID)
	if err != nil {
		t.Fatal(err)
	}

	result, err = resolve()
	if result != nil ||
		!errors.Is(err, ErrDriverPayoutExecutionConflict) {
		t.Fatalf("paid payout accepted: %+v %v", result, err)
	}

	t.Log("PASS: recovery states, immutable identity, claim safety, terminal rejection")
}
