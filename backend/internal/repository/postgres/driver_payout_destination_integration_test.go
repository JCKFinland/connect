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

func TestDriverPayoutDestinationIntegration(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" {
		t.Skip("requires isolated payout destination database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must be disabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	if err := db.QueryRow(ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "connect_payout_destination_test" {
		t.Fatalf("unsafe database: %s", databaseName)
	}

	var version int
	if err := db.QueryRow(ctx, `
        SELECT MAX(version_id)
        FROM goose_db_version
        WHERE is_applied = true
    `).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 74 && version != 75 {
		t.Fatalf("expected migration 74, got %d", version)
	}

	// Use only synthetic records.
	driverID := uuid.NewString()
	companyID := uuid.NewString()
	suffix := uuid.NewString()

	_, err = db.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash,
            first_name, last_name
        )
        VALUES ($1, $2, $3, 'Payout', 'Fixture')
    `, driverID, "payout-"+suffix+"@example.invalid",
		"integration-test-not-a-real-password")
	if err != nil {
		t.Fatalf("create synthetic user: %v", err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, country_code, timezone
        )
        VALUES ($1, $2, $3, 'FI', 'Europe/Helsinki')
    `, companyID,
		"Payout Test "+suffix,
		"Payout Test "+suffix)
	if err != nil {
		t.Fatalf("create synthetic company: %v", err)
	}

	// Cleanup occurs after the test, including failure paths.
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer stop()

		_, _ = db.Exec(cleanupCtx, `
            DELETE FROM driver_payout_executions
            WHERE payout_id IN (
                SELECT id FROM driver_payouts
                WHERE driver_id = $1
            )
        `, driverID)

		_, _ = db.Exec(cleanupCtx,
			"DELETE FROM driver_payouts WHERE driver_id=$1",
			driverID)

		_, _ = db.Exec(cleanupCtx,
			"DELETE FROM driver_payout_destinations WHERE driver_id=$1",
			driverID)

		_, _ = db.Exec(cleanupCtx,
			"DELETE FROM companies WHERE id=$1",
			companyID)

		_, _ = db.Exec(cleanupCtx,
			"DELETE FROM users WHERE id=$1",
			driverID)
	})

	createPayout := func() string {
		t.Helper()
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
                NOW()-INTERVAL '2 days',
                NOW()-INTERVAL '1 day',
                100, 7, 0, 93, 'EUR',
                'BANK_TRANSFER', 'PENDING'
            )
            RETURNING id
        `, driverID, companyID,
			"bound-test-"+uuid.NewString(),
		).Scan(&payoutID)
		if err != nil {
			t.Fatalf("create synthetic payout: %v", err)
		}
		return payoutID
	}

	createDestination := func() string {
		t.Helper()
		var id string
		err := db.QueryRow(ctx, `
            INSERT INTO driver_payout_destinations (
                driver_id, company_id, provider,
                provider_destination_id
            )
            VALUES ($1, $2, 'TEST_PROVIDER', $3)
            RETURNING id
        `, driverID, companyID,
			"test-destination-"+uuid.NewString(),
		).Scan(&id)
		if err != nil {
			t.Fatalf("create destination: %v", err)
		}

		_, err = db.Exec(ctx, `
            UPDATE driver_payout_destinations
            SET verification_status='VERIFIED',
                verified_at=NOW()
            WHERE id=$1
        `, id)
		if err != nil {
			t.Fatalf("verify destination: %v", err)
		}
		return id
	}

	payoutID := createPayout()
	destinationID := createDestination()

	params := CreateBoundDriverPayoutExecutionParams{
		PayoutID:      payoutID,
		Provider:      "TEST_PROVIDER",
		DestinationID: destinationID,
	}

	first, err := CreateBoundDriverPayoutExecution(ctx, db, params)
	if err != nil {
		t.Fatalf("bound creation: %v", err)
	}
	if first == nil || first.ID == "" || first.Status != "PENDING" {
		t.Fatalf("unexpected bound execution: %+v", first)
	}

	var storedDestination string
	if err := db.QueryRow(ctx, `
        SELECT destination_id
        FROM driver_payout_executions
        WHERE payout_id=$1
    `, payoutID).Scan(&storedDestination); err != nil {
		t.Fatal(err)
	}
	if storedDestination != destinationID {
		t.Fatalf("incorrect destination binding: %s", storedDestination)
	}

	replay, err := CreateBoundDriverPayoutExecution(ctx, db, params)
	if err != nil || replay == nil || replay.ID != first.ID {
		t.Fatalf("idempotent replay failed: %+v %v", replay, err)
	}

	otherDestination := createDestination()
	_, err = CreateBoundDriverPayoutExecution(ctx, db,
		CreateBoundDriverPayoutExecutionParams{
			PayoutID:      payoutID,
			Provider:      "TEST_PROVIDER",
			DestinationID: otherDestination,
		})
	if !errors.Is(err, ErrDriverPayoutExecutionConflict) {
		t.Fatalf("expected conflicting destination, got %v", err)
	}

	_, err = CreateDriverPayoutExecution(ctx, db,
		CreateDriverPayoutExecutionParams{
			PayoutID: createPayout(),
			Provider: "TEST_PROVIDER",
		})
	if !errors.Is(err, ErrDriverPayoutExecutionConflict) {
		t.Fatalf("legacy creation should be rejected: %v", err)
	}

	claimed, didClaim, err := ClaimDriverPayoutExecution(ctx, db, payoutID)
	if err != nil || !didClaim || claimed == nil ||
		claimed.Status != "PROCESSING" {
		t.Fatalf("claim failed: %+v claimed=%v err=%v",
			claimed, didClaim, err)
	}

	_, didClaim, err = ClaimDriverPayoutExecution(ctx, db, payoutID)
	if err != nil || didClaim {
		t.Fatalf("claim replay should not resubmit: %v %v",
			didClaim, err)
	}

	revokedPayout := createPayout()
	revokedDestination := createDestination()

	_, err = CreateBoundDriverPayoutExecution(ctx, db,
		CreateBoundDriverPayoutExecutionParams{
			PayoutID:      revokedPayout,
			Provider:      "TEST_PROVIDER",
			DestinationID: revokedDestination,
		})
	if err != nil {
		t.Fatalf("create revocation fixture: %v", err)
	}

	_, err = db.Exec(ctx, `
        UPDATE driver_payout_destinations
        SET verification_status='REVOKED'
        WHERE id=$1
    `, revokedDestination)
	if err != nil {
		t.Fatalf("revoke destination: %v", err)
	}

	_, didClaim, err = ClaimDriverPayoutExecution(
		ctx, db, revokedPayout,
	)
	if !errors.Is(err, ErrDriverPayoutExecutionConflict) ||
		didClaim {
		t.Fatalf("revoked destination was claimable: %v %v",
			didClaim, err)
	}

	t.Log("PASS: bound creation, replay, isolation, claim and revocation")
}
