package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStripeOwnershipReaderPostgresRollback(t *testing.T) {
	const databaseName = "connect_payout_destination_test"

	if os.Getenv("DB_NAME") != databaseName {
		t.Skip("requires isolated payout destination test database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must be disabled")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 60*time.Second,
	)
	defer cancel()

	envOr := func(name, fallback string) string {
		if value := os.Getenv(name); value != "" {
			return value
		}
		return fallback
	}

	cfg, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("DB_HOST", "localhost"),
		envOr("DB_PORT", "5432"),
		envOr("DB_USER", "postgres"),
		databaseName,
	))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2

	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var actualDB string
	var version int
	var tableExists bool

	err = db.QueryRow(ctx, `
        SELECT current_database(),
               (SELECT MAX(version_id)::int
                FROM goose_db_version
                WHERE is_applied),
               to_regclass(
                   'public.stripe_connect_ownership_bindings'
               ) IS NOT NULL
    `).Scan(&actualDB, &version, &tableExists)
	if err != nil {
		t.Fatal(err)
	}
	if actualDB != databaseName || version != 75 || tableExists {
		t.Fatalf(
			"unsafe database: name=%s version=%d table=%v",
			actualDB, version, tableExists,
		)
	}

	migration, err := os.ReadFile(
		"../../../migrations/" +
			"00077_create_stripe_connect_ownership_bindings.sql",
	)
	if err != nil {
		t.Fatal(err)
	}

	source := string(migration)
	if strings.Count(source, "-- +goose Up") != 1 ||
		strings.Count(source, "-- +goose Down") != 1 {
		t.Fatal("invalid migration sections")
	}

	up := strings.SplitN(
		strings.SplitN(source, "-- +goose Up", 2)[1],
		"-- +goose Down", 2,
	)[0]

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}

	rolledBack := false
	defer func() {
		if !rolledBack {
			cleanupCtx, stop := context.WithTimeout(
				context.Background(), 15*time.Second,
			)
			defer stop()
			if err := tx.Rollback(cleanupCtx); err != nil &&
				!errors.Is(err, pgx.ErrTxClosed) {
				t.Errorf("transaction rollback failed: %v", err)
			}
		}
	}()

	if _, err := tx.Exec(
		ctx, "SET LOCAL statement_timeout = '20s'",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(
		ctx, "SET LOCAL lock_timeout = '3s'",
	); err != nil {
		t.Fatal(err)
	}

	// Simple protocol permits the migration's multiple SQL statements.
	if _, err := tx.Exec(
		ctx, up, pgx.QueryExecModeSimpleProtocol,
	); err != nil {
		t.Fatalf("transactional migration failed: %v", err)
	}

	actorID := uuid.NewString()
	driverID := uuid.NewString()
	companyID := uuid.NewString()
	accountID := "acct_" + strings.ReplaceAll(
		uuid.NewString(), "-", "",
	)

	_, err = tx.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash, first_name, last_name
        ) VALUES
          ($1, $2, 'synthetic-test-only', 'Test', 'Actor'),
          ($3, $4, 'synthetic-test-only', 'Test', 'Driver')
    `,
		actorID, "ownership-"+actorID+"@example.invalid",
		driverID, "ownership-"+driverID+"@example.invalid",
	)
	if err != nil {
		t.Fatalf("insert synthetic users: %v", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name, country_code, timezone
        ) VALUES ($1, $2, $2, 'FI', 'Europe/Helsinki')
    `, companyID, "Ownership Test "+companyID)
	if err != nil {
		t.Fatalf("insert synthetic company: %v", err)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO stripe_connect_ownership_bindings (
            actor_user_id, driver_user_id,
            company_id, stripe_account_id
        ) VALUES ($1, $2, $3, $4)
    `, actorID, driverID, companyID, accountID)
	if err != nil {
		t.Fatalf("insert pending binding: %v", err)
	}

	reader := NewStripeOwnershipEvidenceReader(tx)
	request := payouts.StripeConnectOwnershipRequest{
		ActorUserID:  actorID,
		DriverUserID: driverID,
		CompanyID:    companyID,
		AccountID:    accountID,
	}

	cases := []struct {
		name   string
		change func(*payouts.StripeConnectOwnershipRequest)
	}{
		{"pending", func(*payouts.StripeConnectOwnershipRequest) {}},
		{"wrong_actor", func(r *payouts.StripeConnectOwnershipRequest) {
			r.ActorUserID = driverID
		}},
		{"wrong_driver", func(r *payouts.StripeConnectOwnershipRequest) {
			r.DriverUserID = actorID
		}},
		{"wrong_company", func(r *payouts.StripeConnectOwnershipRequest) {
			r.CompanyID = uuid.NewString()
		}},
		{"wrong_account", func(r *payouts.StripeConnectOwnershipRequest) {
			r.AccountID = "acct_" + strings.ReplaceAll(
				uuid.NewString(), "-", "",
			)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := request
			tc.change(&current)

			evidence, err := reader.LookupVerifiedOwnership(
				ctx, current,
			)
			if !errors.Is(
				err, payouts.ErrStripeConnectOwnershipNotVerified,
			) || evidence.Verified {
				t.Fatalf(
					"expected fail-closed rejection, evidence=%+v err=%v",
					evidence, err,
				)
			}
		})
	}

	// Verify the reader did not mutate the pending record.
	var status string
	err = tx.QueryRow(ctx, `
        SELECT verification_status
        FROM stripe_connect_ownership_bindings
        WHERE actor_user_id=$1::uuid
          AND driver_user_id=$2::uuid
          AND company_id=$3::uuid
          AND stripe_account_id=$4
    `, actorID, driverID, companyID, accountID).Scan(&status)
	if err != nil || status != "PENDING" {
		t.Fatalf("binding modified: status=%s err=%v", status, err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	rolledBack = true

	var finalVersion int
	var finalTableExists bool
	err = db.QueryRow(ctx, `
        SELECT
            (SELECT MAX(version_id)::int
             FROM goose_db_version WHERE is_applied),
            to_regclass(
                'public.stripe_connect_ownership_bindings'
            ) IS NOT NULL
    `).Scan(&finalVersion, &finalTableExists)
	if err != nil {
		t.Fatal(err)
	}
	if finalVersion != 75 || finalTableExists {
		t.Fatalf(
			"rollback verification failed: version=%d table=%v",
			finalVersion, finalTableExists,
		)
	}

	t.Log("PASS: actual PostgreSQL reader rejects pending evidence")
	t.Log("PASS: ownership mismatches rejected")
	t.Log("PASS: temporary schema and fixtures rolled back")
}
