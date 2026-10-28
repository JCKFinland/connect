package postgres

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDriverPayoutDestinationConcurrency(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" {
		t.Skip("requires isolated destination test database")
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

	config, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%s user=%s dbname=connect_payout_destination_test sslmode=disable",
		host, port, user,
	))
	if err != nil {
		t.Fatal(err)
	}

	config.MaxConns = 12

	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	if err := db.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	var dbName string
	if err := db.QueryRow(ctx,
		"SELECT current_database()",
	).Scan(&dbName); err != nil {
		t.Fatal(err)
	}

	if dbName != "connect_payout_destination_test" {
		t.Fatalf("unsafe database: %s", dbName)
	}

	var version int
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

	userID := uuid.NewString()
	companyID := uuid.NewString()
	suffix := uuid.NewString()

	_, err = db.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash,
            first_name, last_name
        )
        VALUES ($1, $2, $3, 'Concurrent', 'Fixture')
    `, userID, "concurrent-"+suffix+"@example.invalid",
		"integration-test-only")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, country_code, timezone
        )
        VALUES ($1, $2, $3, 'FI', 'Europe/Helsinki')
    `, companyID, "Concurrency "+suffix,
		"Concurrency "+suffix)
	if err != nil {
		t.Fatal(err)
	}

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
              )`, userID},
			{`DELETE FROM driver_payouts WHERE driver_id=$1`, userID},
			{`DELETE FROM driver_payout_destinations WHERE driver_id=$1`, userID},
			{`DELETE FROM companies WHERE id=$1`, companyID},
			{`DELETE FROM users WHERE id=$1`, userID},
		}

		for _, statement := range statements {
			if _, err := db.Exec(
				cleanupCtx, statement.query, statement.id,
			); err != nil {
				t.Errorf("fixture cleanup: %v", err)
			}
		}
	})

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
            NOW()-INTERVAL '2 days',
            NOW()-INTERVAL '1 day',
            100, 7, 0, 93, 'EUR',
            'BANK_TRANSFER', 'PENDING'
        )
        RETURNING id
    `, userID, companyID,
		"concurrent-payout-"+uuid.NewString(),
	).Scan(&payoutID)
	if err != nil {
		t.Fatal(err)
	}

	var destinationID string
	err = db.QueryRow(ctx, `
        INSERT INTO driver_payout_destinations (
            driver_id, company_id, provider,
            provider_destination_id
        )
        VALUES ($1, $2, 'TEST_PROVIDER', $3)
        RETURNING id
    `, userID, companyID,
		"concurrent-destination-"+uuid.NewString(),
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

	const workers = 8

	params := CreateBoundDriverPayoutExecutionParams{
		PayoutID:      payoutID,
		Provider:      "TEST_PROVIDER",
		DestinationID: destinationID,
	}

	start := make(chan struct{})
	var wg sync.WaitGroup

	results := make([]*DriverPayoutExecution, workers)
	errs := make([]error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], errs[index] =
				CreateBoundDriverPayoutExecution(ctx, db, params)
		}(i)
	}

	close(start)
	wg.Wait()

	var executionID string

	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("creation worker %d: %v", i, errs[i])
		}
		if results[i] == nil {
			t.Fatalf("creation worker %d returned nil", i)
		}
		if executionID == "" {
			executionID = results[i].ID
		}
		if results[i].ID != executionID {
			t.Fatal("concurrent creation produced different executions")
		}
	}

	var executionCount int
	err = db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM driver_payout_executions
        WHERE payout_id=$1
    `, payoutID).Scan(&executionCount)
	if err != nil {
		t.Fatal(err)
	}

	if executionCount != 1 {
		t.Fatalf("expected one execution, got %d", executionCount)
	}

	claimStart := make(chan struct{})
	claimResults := make([]bool, workers)
	claimErrors := make([]error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-claimStart
			_, claimResults[index], claimErrors[index] =
				ClaimDriverPayoutExecution(ctx, db, payoutID)
		}(i)
	}

	close(claimStart)
	wg.Wait()

	successfulClaims := 0

	for i := 0; i < workers; i++ {
		if claimErrors[i] != nil {
			t.Fatalf("claim worker %d: %v", i, claimErrors[i])
		}
		if claimResults[i] {
			successfulClaims++
		}
	}

	if successfulClaims != 1 {
		t.Fatalf("expected one successful claim, got %d",
			successfulClaims)
	}

	var status string
	var attempts int

	err = db.QueryRow(ctx, `
        SELECT execution_status, attempt_count
        FROM driver_payout_executions
        WHERE payout_id=$1
    `, payoutID).Scan(&status, &attempts)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" || attempts != 1 {
		t.Fatalf(
			"invalid final state: status=%s attempts=%d",
			status, attempts,
		)
	}

	t.Logf(
		"PASS: %d concurrent creators, %d concurrent claimers, "+
			"one execution and one submission authorization",
		workers, workers,
	)
}
