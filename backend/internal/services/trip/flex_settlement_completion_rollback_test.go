package trip

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestFlexTripRollsBackWhenSettlementPersistenceFails(t *testing.T) {
	if testing.Short() {
		t.Skip("requires settlement integration database")
	}
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	ctx := context.Background()

	prepared := prepareAuthorizedFlexLifecycle(t)
	f := prepared.Fixture

	var databaseName string
	if err := f.DB.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatalf("identify integration database: %v", err)
	}
	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing settlement failure injection against %q",
			databaseName,
		)
	}

	blocker, err := f.DB.Begin(ctx)
	if err != nil {
		t.Fatalf("begin settlement blocking transaction: %v", err)
	}
	defer func() {
		_ = blocker.Rollback(context.Background())
	}()

	lockKey := "flex-settlement:" + prepared.TripID

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		blocker,
		lockKey,
	); err != nil {
		t.Fatalf("hold settlement advisory lock: %v", err)
	}

	// A bounded context prevents indefinite blocking if the expected
	// settlement lock is not reached.
	completionCtx, cancel := context.WithTimeout(
		ctx,
		3*time.Second,
	)
	defer cancel()

	completedFare, completionErr := prepared.TripService.CompleteTrip(
		completionCtx,
		prepared.TripID,
		f.DriverID,
	)

	if completionErr == nil {
		t.Fatal("expected completion to fail on settlement lock contention")
	}
	if completedFare != nil {
		t.Fatalf("unexpected completed fare: %+v", completedFare)
	}

	// Require evidence that the failure occurred at settlement creation,
	// rather than at an earlier trip lifecycle validation step.
	if !strings.Contains(
		completionErr.Error(),
		"lock FLEX settlement trip",
	) {
		t.Fatalf(
			"completion failed before settlement lock: %v",
			completionErr,
		)
	}

	var pgErr *pgconn.PgError
	if errors.As(completionErr, &pgErr) {
		t.Logf(
			"settlement lock PostgreSQL error: %s",
			pgErr.Code,
		)
	}

	// Release the blocker before inspecting the committed database state.
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("release settlement blocker: %v", err)
	}

	var (
		status         string
		active         bool
		actualDistance *int64
		actualDuration *int64
	)

	if err := f.DB.QueryRow(ctx, `
		SELECT status,
		       is_active,
		       actual_distance_meters,
		       actual_duration_seconds
		FROM trips
		WHERE id = $1
	`, prepared.TripID).Scan(
		&status,
		&active,
		&actualDistance,
		&actualDuration,
	); err != nil {
		t.Fatalf("read trip after settlement rollback: %v", err)
	}

	if status != StatusInProgress || !active {
		t.Fatalf(
			"trip completion persisted after rollback: status=%s active=%v",
			status,
			active,
		)
	}

	if actualDistance != nil || actualDuration != nil {
		t.Fatalf(
			"trip metrics persisted after rollback: distance=%v duration=%v",
			actualDistance,
			actualDuration,
		)
	}

	checks := []struct {
		name  string
		query string
	}{
		{
			name:  "trip fare",
			query: `SELECT count(*) FROM trip_fares WHERE trip_id = $1`,
		},
		{
			name:  "meter snapshot",
			query: `SELECT count(*) FROM trip_meter_measurements WHERE trip_id = $1`,
		},
		{
			name: "completion event",
			query: `SELECT count(*) FROM trip_events
				WHERE trip_id = $1
				AND event_type = 'TRIP_COMPLETED'`,
		},
		{
			name:  "FLEX settlement",
			query: `SELECT count(*) FROM flex_settlements WHERE trip_id = $1`,
		},
		{
			name: "captured payment",
			query: `SELECT count(*) FROM payments
				WHERE trip_id = $1 AND status = 'PAID'`,
		},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			var count int
			if err := f.DB.QueryRow(
				ctx,
				check.query,
				prepared.TripID,
			).Scan(&count); err != nil {
				t.Fatalf("count %s: %v", check.name, err)
			}
			if count != 0 {
				t.Fatalf(
					"%s persisted after rollback: %d",
					check.name,
					count,
				)
			}
		})
	}

	var availabilityStatus string
	if err := f.DB.QueryRow(ctx, `
		SELECT availability_status
		FROM driver_presence
		WHERE driver_id = $1
	`, f.DriverID).Scan(&availabilityStatus); err != nil {
		t.Fatalf("read driver presence: %v", err)
	}

	if availabilityStatus != "BUSY" {
		t.Fatalf(
			"driver released despite settlement rollback: %s",
			availabilityStatus,
		)
	}

	t.Log(
		fmt.Sprintf(
			"atomic rollback verified for FLEX trip %s",
			prepared.TripID,
		),
	)
}
