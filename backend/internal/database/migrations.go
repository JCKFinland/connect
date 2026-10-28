package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/JCKFinland/connect/backend/internal/config"
)

// RunMigrations verifies the production schema without applying migrations.
// In development and test, it applies pending Goose migrations.
func RunMigrations(cfg *config.Config, log *slog.Logger) error {

	connectionConfig, err := pgx.ParseConfig(
		"sslmode=" + cfg.Database.SSLMode,
	)
	if err != nil {
		return fmt.Errorf("invalid migration database configuration: %w", err)
	}

	connectionConfig.Host = cfg.Database.Host
	connectionConfig.User = cfg.Database.User
	connectionConfig.Database = cfg.Database.Name
	connectionConfig.Password = cfg.Database.Password

	port, err := strconv.ParseUint(cfg.Database.Port, 10, 16)
	if err != nil {
		return fmt.Errorf("invalid migration database port: %w", err)
	}
	connectionConfig.Port = uint16(port)

	db := stdlib.OpenDB(*connectionConfig)
	defer db.Close()

	migrationsDir := filepath.Join(".", "migrations")

	if cfg.App.Env == "production" {
		if err := verifyProductionPayoutMigrationGate(db); err != nil {
			return err
		}

		log.Info("Production database schema verified; automatic migrations disabled")
		return nil
	}

	if err := goose.Up(db, migrationsDir); err != nil {
		return err
	}

	log.Info("Database migrations completed successfully")

	return nil
}

// verifyProductionPayoutMigrationGate prevents application startup from
// implicitly deploying payout migrations 74 and 75 in production.
//
// Production schema upgrades crossing this boundary require a separately
// controlled deployment procedure. This check does not run migrations.
func verifyProductionPayoutMigrationGate(db *sql.DB) error {
	if db == nil {
		return errors.New("production payout migration gate: nil database")
	}

	var migration74Applied bool
	var migration75Applied bool

	err := db.QueryRow(`
        SELECT
            EXISTS (
                SELECT 1
                FROM goose_db_version
                WHERE version_id = 74
                  AND is_applied = TRUE
            ),
            EXISTS (
                SELECT 1
                FROM goose_db_version
                WHERE version_id = 75
                  AND is_applied = TRUE
            )
    `).Scan(
		&migration74Applied,
		&migration75Applied,
	)

	if err != nil {
		return fmt.Errorf(
			"production payout migration gate: cannot verify migration history: %w",
			err,
		)
	}

	if !migration74Applied || !migration75Applied {
		return fmt.Errorf(
			"production payout migration gate: migrations 74 and 75 "+
				"must both be applied through controlled deployment "+
				"(migration74=%t migration75=%t)",
			migration74Applied,
			migration75Applied,
		)
	}

	if err := verifyProductionPayoutSchema(db); err != nil {
		return err
	}

	return nil
}

// verifyProductionPayoutSchema checks the structural dependencies required
// by migrations 74 and 75 without changing any database state.
func verifyProductionPayoutSchema(db *sql.DB) error {
	required := []struct {
		name  string
		query string
	}{
		{
			name:  "payout executions table",
			query: "SELECT to_regclass('public.driver_payout_executions') IS NOT NULL",
		},
		{
			name:  "payout destinations table",
			query: "SELECT to_regclass('public.driver_payout_destinations') IS NOT NULL",
		},
		{
			name:  "payout recovery leases table",
			query: "SELECT to_regclass('public.driver_payout_recovery_leases') IS NOT NULL",
		},
		{
			name: "execution destination column",
			query: `SELECT EXISTS (
                SELECT 1 FROM information_schema.columns
                WHERE table_schema = 'public'
                  AND table_name = 'driver_payout_executions'
                  AND column_name = 'destination_id'
                  AND udt_name = 'uuid'
            )`,
		},
		{
			name: "destination integrity trigger",
			query: `SELECT EXISTS (
                SELECT 1 FROM pg_trigger
                WHERE tgrelid = 'public.driver_payout_destinations'::regclass
                  AND tgname = 'trg_protect_driver_payout_destination'
                  AND NOT tgisinternal
                  AND tgenabled <> 'D'
            )`,
		},
		{
			name: "execution destination trigger",
			query: `SELECT EXISTS (
                SELECT 1 FROM pg_trigger
                WHERE tgrelid = 'public.driver_payout_executions'::regclass
                  AND tgname = 'trg_enforce_driver_payout_execution_destination'
                  AND NOT tgisinternal
                  AND tgenabled <> 'D'
            )`,
		},
		{
			name: "recovery lease token",
			query: `SELECT EXISTS (
                SELECT 1 FROM information_schema.columns
                WHERE table_schema = 'public'
                  AND table_name = 'driver_payout_recovery_leases'
                  AND column_name = 'lease_token'
                  AND udt_name = 'uuid'
            )`,
		},
	}

	for _, check := range required {
		var present bool

		if err := db.QueryRow(check.query).Scan(&present); err != nil {
			return fmt.Errorf(
				"production payout schema gate: %s verification failed: %w",
				check.name,
				err,
			)
		}

		if !present {
			return fmt.Errorf(
				"production payout schema gate: missing %s",
				check.name,
			)
		}
	}

	return nil
}
