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

func TestDriverPayoutTransferRequestIntegration(t *testing.T) {
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
        SELECT
            current_database(),
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

	// Register cleanup before creating any synthetic records.
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
                SELECT id FROM driver_payouts
                WHERE driver_id=$1
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
            id, email, password_hash,
            first_name, last_name
        )
        VALUES ($1, $2, $3, 'Resolver', 'Fixture')
    `, driverID, "resolver-"+suffix+"@example.invalid",
		"integration-test-only")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, country_code, timezone
        )
        VALUES ($1, $2, $3, 'FI', 'Europe/Helsinki')
    `, companyID, "Resolver "+suffix, "Resolver "+suffix)
	if err != nil {
		t.Fatal(err)
	}

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
                25.00, 3.35, 0.00, 21.65,
                'EUR', 'BANK_TRANSFER', 'PENDING'
            )
            RETURNING id
        `, driverID, companyID,
			"resolver-"+uuid.NewString(),
		).Scan(&payoutID)
		if err != nil {
			t.Fatal(err)
		}

		return payoutID
	}

	createDestination := func() (string, string) {
		t.Helper()

		externalID := "resolver-destination-" + uuid.NewString()

		var destinationID string
		err := db.QueryRow(ctx, `
            INSERT INTO driver_payout_destinations (
                driver_id, company_id, provider,
                provider_destination_id
            )
            VALUES ($1, $2, 'TEST_PROVIDER', $3)
            RETURNING id
        `, driverID, companyID, externalID).
			Scan(&destinationID)
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

		return destinationID, externalID
	}

	payoutID := createPayout()
	destinationID, externalID := createDestination()

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

	// PENDING is not submission-ready.
	request, err := ResolveDriverPayoutTransferRequest(
		ctx, db, payoutID,
	)
	if request != nil ||
		!errors.Is(err, ErrDriverPayoutExecutionConflict) {
		t.Fatalf(
			"unclaimed payout accepted: %+v %v",
			request, err,
		)
	}

	claimed, didClaim, err := ClaimDriverPayoutExecution(
		ctx, db, payoutID,
	)
	if err != nil || !didClaim || claimed == nil {
		t.Fatalf(
			"claim failed: %+v %v %v",
			claimed, didClaim, err,
		)
	}

	request, err = ResolveDriverPayoutTransferRequest(
		ctx, db, payoutID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if request.PayoutID != payoutID ||
		request.IdempotencyKey != claimed.IdempotencyKey ||
		request.DestinationID != externalID ||
		request.AmountCents != 2165 ||
		request.Currency != "EUR" {
		t.Fatalf("incorrect transfer request: %+v", request)
	}

	// Revocation after claim cannot redirect the transfer.
	_, err = db.Exec(ctx, `
        UPDATE driver_payout_destinations
        SET verification_status='REVOKED'
        WHERE id=$1
    `, destinationID)
	if err != nil {
		t.Fatal(err)
	}

	replay, err := ResolveDriverPayoutTransferRequest(
		ctx, db, payoutID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if *replay != *request {
		t.Fatalf(
			"transfer identity changed after revocation: %+v %+v",
			request, replay,
		)
	}

	// Resolver calls cannot create a second claim.
	_, didClaim, err = ClaimDriverPayoutExecution(
		ctx, db, payoutID,
	)
	if err != nil || didClaim {
		t.Fatalf("second claim authorized: %v %v", didClaim, err)
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

	if attempts != 1 || status != "PROCESSING" {
		t.Fatalf(
			"resolver changed execution state: %d %s",
			attempts, status,
		)
	}

	// Invalid identity must fail before database lookup.
	request, err = ResolveDriverPayoutTransferRequest(
		ctx, db, "invalid-payout-id",
	)
	if request != nil ||
		!errors.Is(err, ErrDriverPayoutExecutionInvalidInput) {
		t.Fatalf("invalid identity accepted: %+v %v", request, err)
	}

	t.Log(
		"PASS: exact cents, bound destination, claim gating,",
		"revocation identity, replay and state immutability",
	)
}
