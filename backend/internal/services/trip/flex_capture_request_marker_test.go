package trip

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func prepareFlexCaptureMarkerFixture(
	t *testing.T,
) (*pgxpool.Pool, string) {
	t.Helper()

	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB
	ctx := context.Background()

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing settlement integration test on %q",
			databaseName,
		)
	}

	settlement, err :=
		postgres.CreateFlexSettlementFromCompletedTrip(
			ctx,
			db,
			lifecycle.TripID,
		)
	if err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	_, claimed, err :=
		postgres.ClaimFlexSettlementProcessing(
			ctx,
			db,
			settlement.ID,
		)
	if err != nil || !claimed {
		t.Fatalf(
			"claim settlement: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	prepared, err :=
		postgres.PrepareFlexSettlementCapture(
			ctx,
			db,
			settlement.ID,
		)
	if err != nil {
		t.Fatalf("prepare settlement: %v", err)
	}

	if prepared == nil ||
		prepared.PaymentID == "" ||
		prepared.CaptureTransactionID == "" {
		t.Fatalf("invalid preparation: %+v", prepared)
	}

	return db, settlement.ID
}

func TestFlexCaptureRequestMarkerFirstAndReplay(t *testing.T) {
	db, settlementID := prepareFlexCaptureMarkerFixture(t)
	ctx := context.Background()

	marked, err :=
		postgres.MarkFlexSettlementCaptureRequested(
			ctx,
			db,
			settlementID,
		)
	if err != nil {
		t.Fatalf("mark first capture request: %v", err)
	}
	if marked == nil || marked.Status != "PROCESSING" {
		t.Fatalf("unexpected marked settlement: %+v", marked)
	}

	var firstTimestamp time.Time

	err = db.QueryRow(ctx, `
		SELECT capture_requested_at
		FROM flex_settlements
		WHERE id = $1
	`, settlementID).Scan(&firstTimestamp)
	if err != nil {
		t.Fatal(err)
	}

	if firstTimestamp.IsZero() {
		t.Fatal("capture request timestamp was not persisted")
	}

	_, err = postgres.MarkFlexSettlementCaptureRequested(
		ctx,
		db,
		settlementID,
	)
	if !errors.Is(err, postgres.ErrFlexCaptureAlreadyRequested) {
		t.Fatalf("expected replay rejection, got %v", err)
	}

	var replayTimestamp time.Time
	err = db.QueryRow(ctx, `
		SELECT capture_requested_at
		FROM flex_settlements
		WHERE id = $1
	`, settlementID).Scan(&replayTimestamp)
	if err != nil {
		t.Fatal(err)
	}

	if !replayTimestamp.Equal(firstTimestamp) {
		t.Fatal("capture request timestamp changed on replay")
	}
}

func TestFlexCaptureRequestMarkerConcurrentAttempts(t *testing.T) {
	db, settlementID := prepareFlexCaptureMarkerFixture(t)
	ctx := context.Background()

	const workers = 8

	var wg sync.WaitGroup
	results := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := postgres.MarkFlexSettlementCaptureRequested(
				ctx,
				db,
				settlementID,
			)
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	successes := 0
	replays := 0

	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, postgres.ErrFlexCaptureAlreadyRequested):
			replays++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}

	if successes != 1 || replays != workers-1 {
		t.Fatalf(
			"expected one success and %d replays, got successes=%d replays=%d",
			workers-1,
			successes,
			replays,
		)
	}
}

func TestFlexCaptureRequestMarkerDatabaseImmutability(t *testing.T) {
	db, settlementID := prepareFlexCaptureMarkerFixture(t)
	ctx := context.Background()

	_, err := postgres.MarkFlexSettlementCaptureRequested(
		ctx,
		db,
		settlementID,
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		query string
	}{
		{
			name: "clear timestamp",
			query: `
				UPDATE flex_settlements
				SET capture_requested_at = NULL
				WHERE id = $1
			`,
		},
		{
			name: "replace timestamp",
			query: `
				UPDATE flex_settlements
				SET capture_requested_at =
					capture_requested_at + INTERVAL '1 second'
				WHERE id = $1
			`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.Exec(ctx, tc.query, settlementID)

			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) ||
				pgErr.Code != "23514" {
				t.Fatalf(
					"expected database immutability rejection 23514, got %v",
					err,
				)
			}
		})
	}
}

// TestFlexCaptureRequestMarkerRequiresPreparation verifies that
// capture execution cannot be authorized before local financial
// records are prepared.
func TestFlexCaptureRequestMarkerRequiresPreparation(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB
	ctx := context.Background()

	var databaseName string
	if err := db.QueryRow(
		ctx,
		"SELECT current_database()",
	).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}

	if databaseName != "connect_settlement_test" {
		t.Fatalf(
			"refusing settlement integration test on %q",
			databaseName,
		)
	}

	settlement, err :=
		postgres.CreateFlexSettlementFromCompletedTrip(
			ctx,
			db,
			lifecycle.TripID,
		)
	if err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	t.Run("pending settlement", func(t *testing.T) {
		_, err := postgres.MarkFlexSettlementCaptureRequested(
			ctx,
			db,
			settlement.ID,
		)

		if !errors.Is(err, postgres.ErrFlexSettlementInvalidState) {
			t.Fatalf(
				"expected pending-state rejection, got %v",
				err,
			)
		}
	})

	_, claimed, err :=
		postgres.ClaimFlexSettlementProcessing(
			ctx,
			db,
			settlement.ID,
		)

	if err != nil || !claimed {
		t.Fatalf(
			"claim settlement: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	t.Run("processing without financial preparation", func(t *testing.T) {
		_, err := postgres.MarkFlexSettlementCaptureRequested(
			ctx,
			db,
			settlement.ID,
		)

		if !errors.Is(err, postgres.ErrFlexSettlementInvalidState) {
			t.Fatalf(
				"expected unprepared-state rejection, got %v",
				err,
			)
		}
	})

	var captureRequestedAt *time.Time

	err = db.QueryRow(ctx, `
		SELECT capture_requested_at
		FROM flex_settlements
		WHERE id = $1
	`, settlement.ID).Scan(&captureRequestedAt)

	if err != nil {
		t.Fatal(err)
	}

	if captureRequestedAt != nil {
		t.Fatal("capture marker unexpectedly persisted")
	}

	t.Run("database rejects unprepared marker", func(t *testing.T) {
		_, err := db.Exec(ctx, `
			UPDATE flex_settlements
			SET capture_requested_at = clock_timestamp()
			WHERE id = $1
		`, settlement.ID)

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) ||
			pgErr.Code != "23514" {
			t.Fatalf(
				"expected preparation constraint rejection 23514, got %v",
				err,
			)
		}
	})
}
