package driver_payout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type submissionTestProvider struct {
	mu       sync.Mutex
	calls    int
	requests []payouts.TransferRequest
	result   payouts.TransferResult
	err      error
	cancel   context.CancelFunc
}

func (p *submissionTestProvider) InitiateTransfer(
	_ context.Context,
	request payouts.TransferRequest,
) (payouts.TransferResult, error) {
	p.mu.Lock()
	p.calls++
	p.requests = append(p.requests, request)
	result := p.result
	providerErr := p.err
	cancel := p.cancel
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if result.Status == "" && providerErr == nil {
		result.Status = payouts.TransferPending
	}

	return result, providerErr
}

func (p *submissionTestProvider) LookupTransfer(
	context.Context,
	payouts.TransferLookup,
) (payouts.TransferResult, error) {
	return payouts.TransferResult{}, errors.New(
		"lookup forbidden during submission test",
	)
}

type submissionFixture struct {
	ctx                 context.Context
	db                  *pgxpool.Pool
	payoutID            string
	externalDestination string
}

func newSubmissionFixture(t *testing.T) submissionFixture {
	t.Helper()

	if os.Getenv("DB_NAME") != "connect_payout_destination_test" {
		t.Skip("requires isolated payout destination database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must be disabled")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 90*time.Second,
	)
	t.Cleanup(cancel)

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

	cfg.MaxConns = 12

	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	if err := db.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	var databaseName string
	var version int

	err = db.QueryRow(ctx, `
        SELECT current_database(),
               (SELECT MAX(version_id)
                FROM goose_db_version
                WHERE is_applied=true)
    `).Scan(&databaseName, &version)
	if err != nil {
		t.Fatal(err)
	}
	if databaseName != "connect_payout_destination_test" ||
		(version != 74 && version != 75) {
		t.Fatalf(
			"unsafe database: %s migration %d",
			databaseName, version,
		)
	}

	driverID := uuid.NewString()
	companyID := uuid.NewString()
	suffix := uuid.NewString()

	// Cleanup is registered before fixture creation.
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
        VALUES ($1, $2, $3, 'Submit', 'Fixture')
    `, driverID, "submit-"+suffix+"@example.invalid",
		"integration-test-only")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, country_code, timezone
        )
        VALUES ($1, $2, $3, 'FI', 'Europe/Helsinki')
    `, companyID, "Submit "+suffix, "Submit "+suffix)
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
            100, 7, 0, 93,
            'EUR', 'BANK_TRANSFER', 'PENDING'
        )
        RETURNING id
    `, driverID, companyID,
		"submit-"+uuid.NewString(),
	).Scan(&payoutID)
	if err != nil {
		t.Fatal(err)
	}

	externalDestination := "submit-destination-" + uuid.NewString()

	var destinationID string

	err = db.QueryRow(ctx, `
        INSERT INTO driver_payout_destinations (
            driver_id, company_id, provider,
            provider_destination_id
        )
        VALUES ($1, $2, 'TEST_PROVIDER', $3)
        RETURNING id
    `, driverID, companyID, externalDestination).
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

	_, err = postgresrepo.CreateBoundDriverPayoutExecution(
		ctx, db,
		postgresrepo.CreateBoundDriverPayoutExecutionParams{
			PayoutID:      payoutID,
			Provider:      "TEST_PROVIDER",
			DestinationID: destinationID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return submissionFixture{
		ctx:                 ctx,
		db:                  db,
		payoutID:            payoutID,
		externalDestination: externalDestination,
	}
}

func TestSubmitPayoutConcurrentSingleInitiation(t *testing.T) {
	fixture := newSubmissionFixture(t)
	ctx := fixture.ctx
	db := fixture.db
	payoutID := fixture.payoutID
	externalDestination := fixture.externalDestination

	provider := &submissionTestProvider{}

	submitter, err := NewSubmitter(db, provider)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 8

	start := make(chan struct{})
	results := make([]error, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()
			<-start
			results[index] = submitter.SubmitPayout(
				ctx, payoutID,
			)
		}(i)
	}

	close(start)
	wg.Wait()

	unresolved := 0
	unauthorized := 0

	for i, result := range results {
		switch {
		case errors.Is(result, ErrSubmissionUnresolved):
			unresolved++
		case errors.Is(result, ErrSubmissionNotAuthorized):
			unauthorized++
		default:
			t.Fatalf(
				"worker %d returned unexpected result: %v",
				i, result,
			)
		}
	}

	provider.mu.Lock()
	calls := provider.calls
	requests := append(
		[]payouts.TransferRequest(nil),
		provider.requests...,
	)
	provider.mu.Unlock()

	if calls != 1 || len(requests) != 1 {
		t.Fatalf(
			"expected one provider initiation, got %d",
			calls,
		)
	}

	if unresolved != 1 || unauthorized != workers-1 {
		t.Fatalf(
			"unexpected submission results: unresolved=%d unauthorized=%d",
			unresolved, unauthorized,
		)
	}

	request := requests[0]

	if request.PayoutID != payoutID ||
		request.DestinationID != externalDestination ||
		request.AmountCents != 9300 ||
		request.Currency != "EUR" {
		t.Fatalf(
			"incorrect provider request: %+v",
			request,
		)
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

	if status != "REQUIRES_RECONCILIATION" ||
		attempts != 1 {
		t.Fatalf(
			"unexpected execution state: %s attempts=%d",
			status, attempts,
		)
	}

	// A later invocation must not initiate again.
	err = submitter.SubmitPayout(ctx, payoutID)

	if !errors.Is(err, ErrSubmissionNotAuthorized) {
		t.Fatalf("replay was authorized: %v", err)
	}

	provider.mu.Lock()
	finalCalls := provider.calls
	provider.mu.Unlock()

	if finalCalls != 1 {
		t.Fatalf(
			"replay contacted provider: calls=%d",
			finalCalls,
		)
	}

	t.Log(
		"PASS: eight concurrent callers, one provider initiation,",
		"one recorded attempt, reconciliation required",
	)
}
