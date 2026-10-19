package trip

import (
	"context"
	"errors"
	"os"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestFlexSettlementTransactionBoundCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL integration database")
	}
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	lifecycle := completeAuthorizedFlexLifecycle(t)
	db := lifecycle.Fixture.DB
	ctx := context.Background()

	t.Run("rollback preserves committed settlement", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		settlement, err :=
			postgresrepo.CreateFlexSettlementFromCompletedTripTx(
				ctx,
				tx,
				lifecycle.TripID,
			)
		if err != nil {
			t.Fatalf("create settlement in transaction: %v", err)
		}
		if settlement == nil || settlement.Status != "PENDING" {
			t.Fatalf("unexpected settlement: %+v", settlement)
		}

		if err := tx.Rollback(ctx); err != nil {
			t.Fatalf("rollback transaction: %v", err)
		}

		var count int
		err = db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM flex_settlements
			WHERE trip_id = $1
		`, lifecycle.TripID).Scan(&count)
		if err != nil {
			t.Fatalf("count settlements: %v", err)
		}
		if count != 1 {
			t.Fatalf(
				"expected one previously committed settlement after rollback, got %d",
				count,
			)
		}
	})

	t.Run("commit preserves idempotent identity", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		created, err :=
			postgresrepo.CreateFlexSettlementFromCompletedTripTx(
				ctx,
				tx,
				lifecycle.TripID,
			)
		if err != nil {
			t.Fatalf("create settlement in transaction: %v", err)
		}

		repeated, err :=
			postgresrepo.CreateFlexSettlementFromCompletedTripTx(
				ctx,
				tx,
				lifecycle.TripID,
			)
		if err != nil {
			t.Fatalf("repeat creation in transaction: %v", err)
		}
		if repeated.ID != created.ID {
			t.Fatalf("transactional replay changed settlement ID")
		}

		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit settlement: %v", err)
		}

		existing, err :=
			postgresrepo.CreateFlexSettlementFromCompletedTrip(
				ctx,
				db,
				lifecycle.TripID,
			)
		if err != nil {
			t.Fatalf("read settlement through wrapper: %v", err)
		}
		if existing.ID != created.ID {
			t.Fatalf("wrapper changed committed settlement identity")
		}

		var count int
		err = db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM flex_settlements
			WHERE trip_id = $1
		`, lifecycle.TripID).Scan(&count)
		if err != nil {
			t.Fatalf("count committed settlements: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected one settlement, got %d", count)
		}
	})

	t.Run("nil transaction rejected", func(t *testing.T) {
		_, err :=
			postgresrepo.CreateFlexSettlementFromCompletedTripTx(
				ctx,
				nil,
				lifecycle.TripID,
			)
		if err == nil {
			t.Fatal("expected nil transaction rejection")
		}
	})

	t.Run("invalid trip ID rejected", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		_, err =
			postgresrepo.CreateFlexSettlementFromCompletedTripTx(
				ctx,
				tx,
				"invalid-trip-id",
			)
		if !errors.Is(
			err,
			postgresrepo.ErrFlexSettlementNotEligible,
		) {
			t.Fatalf("unexpected invalid trip error: %v", err)
		}
	})

}
