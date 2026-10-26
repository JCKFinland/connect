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

func openPayoutTestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if os.Getenv("DB_NAME") != "connect_settlement_test" {
		t.Fatal("payout integration tests require connect_settlement_test")
	}

	ctx := context.Background()

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

	config, err := pgxpool.ParseConfig(
		fmt.Sprintf(
			"host=%s port=%s user=%s dbname=connect_settlement_test sslmode=disable",
			host,
			port,
			user,
		),
	)
	if err != nil {
		t.Fatalf("parse test database configuration: %v", err)
	}

	config.MaxConns = 4

	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}

	t.Cleanup(db.Close)

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	return db
}

func TestPrepareFlexDriverPayoutNoEligibleEarnings(t *testing.T) {
	ctx := context.Background()
	db := openPayoutTestDatabase(t)

	driverID := uuid.NewString()
	companyID := uuid.NewString()
	reference := "connect-payout-empty-" + uuid.NewString()

	now := time.Now().UTC()

	var before int64

	err := db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM driver_payouts
		WHERE payout_reference = $1
	`, reference).Scan(&before)

	if err != nil {
		t.Fatalf("count payouts before preparation: %v", err)
	}

	if before != 0 {
		t.Fatal("unexpected existing payout reference")
	}

	result, err := PrepareFlexDriverPayout(
		ctx,
		db,
		PrepareDriverPayoutParams{
			DriverID:        driverID,
			CompanyID:       companyID,
			PayoutReference: reference,
			PeriodStart:     now.Add(-24 * time.Hour),
			PeriodEnd:       now,
		},
	)

	if !errors.Is(err, ErrDriverPayoutNoEligibleEarnings) {
		t.Fatalf(
			"expected no eligible earnings, got result=%+v error=%v",
			result,
			err,
		)
	}

	if result != nil {
		t.Fatalf("expected nil payout, got %+v", result)
	}

	var after int64

	err = db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM driver_payouts
		WHERE payout_reference = $1
	`, reference).Scan(&after)

	if err != nil {
		t.Fatalf("count payouts after preparation: %v", err)
	}

	if after != 0 {
		t.Fatalf(
			"payout created despite no eligible earnings: %d",
			after,
		)
	}
}
