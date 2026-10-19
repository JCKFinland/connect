package flex_settlement

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupExecutionGuardTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip(
			"isolated settlement integration tests are disabled",
		)
	}

	if os.Getenv("DB_NAME") != "connect_settlement_test" {
		t.Fatal(
			"DB_NAME must be connect_settlement_test",
		)
	}

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

	password := os.Getenv("DB_PASSWORD")

	config, err := pgxpool.ParseConfig("")
	if err != nil {
		t.Fatalf("parse PostgreSQL configuration: %v", err)
	}

	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		t.Fatalf("invalid PostgreSQL port: %q", port)
	}

	config.ConnConfig.Host = host
	config.ConnConfig.Port = uint16(portNumber)
	config.ConnConfig.User = user
	config.ConnConfig.Password = password
	config.ConnConfig.Database = "connect_settlement_test"

	config.MaxConns = 4

	db, err := pgxpool.NewWithConfig(
		context.Background(),
		config,
	)
	if err != nil {
		t.Fatalf("connect isolated database: %v", err)
	}

	t.Cleanup(db.Close)

	var databaseName string
	err = db.QueryRow(
		context.Background(),
		"SELECT current_database()",
	).Scan(&databaseName)
	if err != nil {
		t.Fatalf("verify isolated database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"unsafe database identity: %q",
			databaseName,
		)
	}

	return db
}

func TestExecutionGuardConcurrentWorkerExclusion(t *testing.T) {
	db := setupExecutionGuardTestDB(t)

	guard := NewExecutionGuard(db)
	settlementID := uuid.NewString()

	entered := make(chan struct{})
	release := make(chan struct{})
	firstResult := make(chan error, 1)

	go func() {
		firstResult <- guard.WithSettlementLock(
			context.Background(),
			settlementID,
			func(context.Context) error {
				close(entered)
				<-release
				return nil
			},
		)
	}()

	select {
	case <-entered:
	case err := <-firstResult:
		t.Fatalf("first worker exited before acquiring lock: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("first worker did not acquire lock")
	}

	secondExecuted := false

	secondErr := guard.WithSettlementLock(
		context.Background(),
		settlementID,
		func(context.Context) error {
			secondExecuted = true
			return nil
		},
	)

	close(release)

	if !errors.Is(
		secondErr,
		ErrSettlementExecutionBusy,
	) {
		t.Fatalf(
			"second worker error = %v, want busy",
			secondErr,
		)
	}

	if secondExecuted {
		t.Fatal("second worker executed while lock was held")
	}

	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("first worker failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first worker did not finish")
	}

	err := guard.WithSettlementLock(
		context.Background(),
		settlementID,
		func(context.Context) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("lock not reusable after release: %v", err)
	}
}

func TestExecutionGuardReleasesLockAfterCallbackError(t *testing.T) {
	db := setupExecutionGuardTestDB(t)

	guard := NewExecutionGuard(db)
	settlementID := uuid.NewString()

	callbackErr := errors.New("simulated execution failure")

	err := guard.WithSettlementLock(
		context.Background(),
		settlementID,
		func(context.Context) error {
			return callbackErr
		},
	)

	if !errors.Is(err, callbackErr) {
		t.Fatalf("callback error not preserved: %v", err)
	}

	err = guard.WithSettlementLock(
		context.Background(),
		settlementID,
		func(context.Context) error {
			return nil
		},
	)

	if err != nil {
		t.Fatalf(
			"lock not released after callback error: %v",
			err,
		)
	}
}

func TestExecutionGuardReleasesLockAfterContextCancellation(
	t *testing.T,
) {
	db := setupExecutionGuardTestDB(t)

	guard := NewExecutionGuard(db)
	settlementID := uuid.NewString()

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	callbackErr := errors.New("callback cancelled")

	err := guard.WithSettlementLock(
		ctx,
		settlementID,
		func(context.Context) error {
			cancel()
			return callbackErr
		},
	)

	if !errors.Is(err, callbackErr) {
		t.Fatalf(
			"cancelled callback error not preserved: %v",
			err,
		)
	}

	err = guard.WithSettlementLock(
		context.Background(),
		settlementID,
		func(context.Context) error {
			return nil
		},
	)

	if err != nil {
		t.Fatalf(
			"lock not released after cancellation: %v",
			err,
		)
	}
}

func TestExecutionGuardIndependentSettlements(t *testing.T) {
	db := setupExecutionGuardTestDB(t)

	guard := NewExecutionGuard(db)

	firstID := uuid.NewString()
	secondID := uuid.NewString()

	var wg sync.WaitGroup
	wg.Add(2)

	errorsCh := make(chan error, 2)

	for _, id := range []string{firstID, secondID} {
		id := id

		go func() {
			defer wg.Done()

			errorsCh <- guard.WithSettlementLock(
				context.Background(),
				id,
				func(context.Context) error {
					time.Sleep(50 * time.Millisecond)
					return nil
				},
			)
		}()
	}

	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
		if err != nil {
			t.Fatalf(
				"independent settlement execution failed: %v",
				err,
			)
		}
	}
}

func TestExecutionGuardNestedSameSettlement(t *testing.T) {
	db := setupExecutionGuardTestDB(t)
	guard := NewExecutionGuard(db)
	settlementID := uuid.NewString()

	nestedExecuted := false

	err := guard.WithSettlementLock(
		context.Background(),
		settlementID,
		func(ctx context.Context) error {
			nestedErr := guard.WithSettlementLock(
				ctx,
				settlementID,
				func(context.Context) error {
					nestedExecuted = true
					return nil
				},
			)

			if !errors.Is(
				nestedErr,
				ErrSettlementExecutionBusy,
			) {
				return fmt.Errorf(
					"nested execution error = %v, want busy",
					nestedErr,
				)
			}

			return nil
		},
	)

	if err != nil {
		t.Fatalf("outer execution failed: %v", err)
	}

	if nestedExecuted {
		t.Fatal("nested callback unexpectedly executed")
	}

	err = guard.WithSettlementLock(
		context.Background(),
		settlementID,
		func(context.Context) error {
			return nil
		},
	)

	if err != nil {
		t.Fatalf("lock not reusable: %v", err)
	}
}

func TestExecutionGuardNestedSingleConnectionPool(t *testing.T) {
	db := setupExecutionGuardTestDB(t)

	// Create a separate pool with exactly one connection.
	// Changing db.Config() alone would not resize an active pool.
	config := db.Config()
	config.MaxConns = 1

	singleDB, err := pgxpool.NewWithConfig(
		context.Background(),
		config,
	)
	if err != nil {
		t.Fatalf("create single-connection pool: %v", err)
	}
	t.Cleanup(singleDB.Close)

	if singleDB.Config().MaxConns != 1 {
		t.Fatal("single-connection pool limit not enforced")
	}

	var databaseName string
	if err := singleDB.QueryRow(
		context.Background(),
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify single-connection database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe database identity: %q", databaseName)
	}

	guard := NewExecutionGuard(singleDB)
	settlementID := uuid.NewString()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	err = guard.WithSettlementLock(
		ctx,
		settlementID,
		func(ctx context.Context) error {
			nestedErr := guard.WithSettlementLock(
				ctx,
				settlementID,
				func(context.Context) error {
					return errors.New(
						"nested callback must not execute",
					)
				},
			)

			if !errors.Is(
				nestedErr,
				ErrSettlementExecutionBusy,
			) {
				return fmt.Errorf(
					"nested error = %v, want busy",
					nestedErr,
				)
			}

			return nil
		},
	)

	if err != nil {
		t.Fatalf(
			"single-connection nested execution failed: %v",
			err,
		)
	}
}

func TestExecutionGuardNestedDifferentSettlement(t *testing.T) {
	db := setupExecutionGuardTestDB(t)

	guard := NewExecutionGuard(db)
	firstID := uuid.NewString()
	secondID := uuid.NewString()

	secondExecuted := false

	err := guard.WithSettlementLock(
		context.Background(),
		firstID,
		func(ctx context.Context) error {
			return guard.WithSettlementLock(
				ctx,
				secondID,
				func(context.Context) error {
					secondExecuted = true
					return nil
				},
			)
		},
	)

	if err != nil {
		t.Fatalf(
			"different-settlement nested execution failed: %v",
			err,
		)
	}

	if !secondExecuted {
		t.Fatal("different-settlement callback did not execute")
	}
}

func TestExecutionGuardCleanupFailurePreservesCallbackError(
	t *testing.T,
) {
	db := setupExecutionGuardTestDB(t)

	// The guard uses a dedicated, uniquely marked session.
	// Only this session may be terminated by this test.
	applicationName := "connect-flex-guard-test-" + uuid.NewString()

	config := db.Config()
	config.MaxConns = 1
	config.ConnConfig.RuntimeParams["application_name"] = applicationName

	guardDB, err := pgxpool.NewWithConfig(
		context.Background(),
		config,
	)
	if err != nil {
		t.Fatalf("create isolated guard pool: %v", err)
	}
	t.Cleanup(guardDB.Close)

	var databaseName string
	if err := guardDB.QueryRow(
		context.Background(),
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("verify guard database: %v", err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf("unsafe guard database: %q", databaseName)
	}

	guard := NewExecutionGuard(guardDB)
	settlementID := uuid.NewString()

	callbackErr := errors.New(
		"simulated provider execution failure",
	)

	err = guard.WithSettlementLock(
		context.Background(),
		settlementID,
		func(ctx context.Context) error {
			// The execution guard holds a dedicated connection.
			// Locate the session holding this settlement lock
			// through a unique application_name marker.
			//
			// A separate database connection terminates that
			// backend to simulate a lost PostgreSQL session.

			var terminated bool

			terminateCtx, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			defer cancel()

			terminateErr := db.QueryRow(
				terminateCtx,
				`
                SELECT pg_terminate_backend(a.pid)
                FROM pg_stat_activity AS a
                WHERE a.datname = 'connect_settlement_test'
                  AND current_database() = 'connect_settlement_test'
                  AND a.application_name = $1
                  AND a.pid <> pg_backend_pid()
                  AND EXISTS (
                      SELECT 1
                      FROM pg_locks AS l
                      WHERE l.pid = a.pid
                        AND l.locktype = 'advisory'
                        AND l.granted
                        AND l.classid = (
                            hashtextextended(
                                'connect:flex-settlement:' || $2::text,
                                0
                            ) >> 32
                        )::int4::oid
                        AND l.objid = (
                            hashtextextended(
                                'connect:flex-settlement:' || $2::text,
                                0
                            ) & 4294967295
                        )::oid
                  )
                `,
				applicationName, settlementID).Scan(&terminated)

			if terminateErr != nil {
				return fmt.Errorf(
					"terminate advisory-lock session: %w",
					terminateErr,
				)
			}

			if !terminated {
				return errors.New(
					"failed to terminate advisory-lock session",
				)
			}

			return callbackErr
		},
	)

	if !errors.Is(err, callbackErr) {
		t.Fatalf(
			"callback error not preserved: %v",
			err,
		)
	}

	if err == callbackErr {
		t.Fatal(
			"expected cleanup failure to be joined with callback error",
		)
	}

	retryCtx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	err = guard.WithSettlementLock(
		retryCtx,
		settlementID,
		func(context.Context) error {
			return nil
		},
	)

	if err != nil {
		t.Fatalf(
			"healthy connection could not reacquire settlement lock: %v",
			err,
		)
	}
}
