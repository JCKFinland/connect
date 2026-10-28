package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDriverPayoutDestinationClaimFirstRace(t *testing.T) {
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

	// Hold the payout lock to make the claim wait before it
	// reaches the destination. Release it, then use a second
	// transaction to verify that claim authorization wins.
	gate, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	gateOpen := true
	defer func() {
		if gateOpen {
			rollbackCtx, stop := context.WithTimeout(
				context.Background(), 5*time.Second,
			)
			defer stop()
			_ = gate.Rollback(rollbackCtx)
		}
	}()

	_, err = gate.Exec(ctx, `
        SELECT id
        FROM driver_payouts
        WHERE id=$1
        FOR UPDATE
    `, payoutID)
	if err != nil {
		t.Fatal(err)
	}

	type claimResult struct {
		claimed bool
		err     error
	}

	claimDone := make(chan claimResult, 1)

	go func() {
		_, claimed, claimErr := ClaimDriverPayoutExecution(
			ctx, db, payoutID,
		)
		claimDone <- claimResult{
			claimed: claimed,
			err:     claimErr,
		}
	}()

	// Claim is initially held behind the payout row lock.
	select {
	case result := <-claimDone:
		t.Fatalf(
			"claim unexpectedly completed while payout locked: %+v",
			result,
		)
	case <-time.After(250 * time.Millisecond):
	}

	if err := gate.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	gateOpen = false

	// A successful claim commits its PROCESSING transition
	// before returning authorization to the caller.
	select {
	case result := <-claimDone:
		if result.err != nil {
			t.Fatalf("claim failed: %v", result.err)
		}
		if !result.claimed {
			t.Fatal("claim did not authorize submission")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("claim timed out")
	}

	// Revocation after a committed claim is permitted,
	// but must not rewrite the historical execution.
	tag, err := db.Exec(ctx, `
        UPDATE driver_payout_destinations
        SET verification_status='REVOKED',
            updated_at=NOW()
        WHERE id=$1
          AND verification_status='VERIFIED'
    `, destinationID)
	if err != nil {
		t.Fatalf("revoke after claim: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected one revoked destination, got %d",
			tag.RowsAffected())
	}

	var status string
	var attempts int
	var started bool
	var verification string
	var boundDestination string

	err = db.QueryRow(ctx, `
        SELECT
            e.execution_status,
            e.attempt_count,
            e.request_started_at IS NOT NULL,
            d.verification_status,
            e.destination_id::text
        FROM driver_payout_executions e
        JOIN driver_payout_destinations d
          ON d.id=e.destination_id
        WHERE e.payout_id=$1
    `, payoutID).Scan(
		&status,
		&attempts,
		&started,
		&verification,
		&boundDestination,
	)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" ||
		attempts != 1 ||
		!started ||
		verification != "REVOKED" ||
		boundDestination != destinationID {
		t.Fatalf(
			"invalid claim-first state: status=%s attempts=%d "+
				"started=%v verification=%s destination=%s",
			status, attempts, started,
			verification, boundDestination,
		)
	}

	// Replay must never grant a second submission.
	_, claimedAgain, err := ClaimDriverPayoutExecution(
		ctx, db, payoutID,
	)
	if err != nil {
		t.Fatalf("claim replay: %v", err)
	}
	if claimedAgain {
		t.Fatal("duplicate provider submission authorization")
	}

	t.Log(
		"PASS: claim authorized once, later revocation preserved " +
			"execution identity and attempt count",
	)
}
