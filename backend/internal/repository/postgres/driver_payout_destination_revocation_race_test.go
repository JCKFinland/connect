package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDriverPayoutDestinationRevocationClaimRace(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" {
		t.Skip("requires isolated destination test database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must be disabled")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 60*time.Second,
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
	cfg.MaxConns = 5

	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	if err := db.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	var dbName string
	var version int
	if err := db.QueryRow(ctx,
		"SELECT current_database()",
	).Scan(&dbName); err != nil {
		t.Fatal(err)
	}
	if dbName != "connect_payout_destination_test" {
		t.Fatalf("unsafe database: %s", dbName)
	}
	if err := db.QueryRow(ctx, `
        SELECT MAX(version_id)
        FROM goose_db_version
        WHERE is_applied=true
    `).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 74 && version != 75 {
		t.Fatalf("expected migration 74, got %d", version)
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
			{`DELETE FROM driver_payouts WHERE driver_id=$1`, driverID},
			{`DELETE FROM driver_payout_destinations
              WHERE driver_id=$1`, driverID},
			{`DELETE FROM companies WHERE id=$1`, companyID},
			{`DELETE FROM users WHERE id=$1`, driverID},
		}
		for _, s := range statements {
			if _, err := db.Exec(
				cleanupCtx, s.query, s.id,
			); err != nil {
				t.Errorf("fixture cleanup: %v", err)
			}
		}
	})

	_, err = db.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash,
            first_name, last_name
        )
        VALUES ($1, $2, 'test-only', 'Race', 'Fixture')
    `, driverID, "revocation-"+suffix+"@example.invalid")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, country_code, timezone
        )
        VALUES ($1, $2, $2, 'FI', 'Europe/Helsinki')
    `, companyID, "Revocation Race "+suffix)
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
            100, 7, 0, 93, 'EUR',
            'BANK_TRANSFER', 'PENDING'
        )
        RETURNING id
    `, driverID, companyID,
		"revocation-payout-"+uuid.NewString(),
	).Scan(&payoutID)
	if err != nil {
		t.Fatal(err)
	}

	var destinationID string
	err = db.QueryRow(ctx, `
        INSERT INTO driver_payout_destinations (
            driver_id, company_id,
            provider, provider_destination_id
        )
        VALUES ($1, $2, 'TEST_PROVIDER', $3)
        RETURNING id
    `, driverID, companyID,
		"revocation-destination-"+uuid.NewString(),
	).Scan(&destinationID)
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

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		rollbackCtx, stop := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer stop()
		_ = tx.Rollback(rollbackCtx)
	}()

	_, err = tx.Exec(ctx, `
        UPDATE driver_payout_destinations
        SET verification_status='REVOKED',
            updated_at=NOW()
        WHERE id=$1
          AND verification_status='VERIFIED'
    `, destinationID)
	if err != nil {
		t.Fatal(err)
	}

	type claimResult struct {
		claimed bool
		err     error
	}
	done := make(chan claimResult, 1)

	go func() {
		_, claimed, claimErr := ClaimDriverPayoutExecution(
			ctx, db, payoutID,
		)
		done <- claimResult{claimed: claimed, err: claimErr}
	}()

	// While revocation holds the destination row lock,
	// the claim must not return a submission authorization.
	select {
	case result := <-done:
		t.Fatalf(
			"claim completed before revocation commit: claimed=%v err=%v",
			result.claimed, result.err,
		)
	case <-time.After(250 * time.Millisecond):
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit revocation: %v", err)
	}

	select {
	case result := <-done:
		if result.claimed {
			t.Fatal("revoked destination authorized a payout claim")
		}
		if !errors.Is(
			result.err, ErrDriverPayoutExecutionConflict,
		) {
			t.Fatalf(
				"expected destination conflict, got %v",
				result.err,
			)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("claim did not complete after revocation")
	}

	var status string
	var attempts int
	var started bool
	var verification string

	err = db.QueryRow(ctx, `
        SELECT
            e.execution_status,
            e.attempt_count,
            e.request_started_at IS NOT NULL,
            d.verification_status
        FROM driver_payout_executions e
        JOIN driver_payout_destinations d
          ON d.id=e.destination_id
        WHERE e.payout_id=$1
    `, payoutID).Scan(
		&status, &attempts, &started, &verification,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PENDING" ||
		attempts != 0 ||
		started ||
		verification != "REVOKED" {
		t.Fatalf(
			"unsafe state: status=%s attempts=%d started=%v destination=%s",
			status, attempts, started, verification,
		)
	}

	t.Log(
		"PASS: revocation commits before claim; " +
			"claim rejected; no submission authorization",
	)
}
