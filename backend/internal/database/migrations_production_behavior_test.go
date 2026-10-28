package database

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestProductionRunMigrationsDoesNotApplyPendingMigrations(t *testing.T) {
	if os.Getenv("CONNECT_PAYOUT_MIGRATION_GATE_TEST") != "1" {
		t.Skip("requires explicitly enabled isolated payout database")
	}

	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	name := os.Getenv("DB_NAME")
	sslmode := os.Getenv("DB_SSLMODE")

	if name != "connect_payout_destination_test" {
		t.Fatal("refusing non-isolated database")
	}

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=%s",
		host, port, user, name, sslmode,
	)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var before int64

	err = db.QueryRow(`
		SELECT COALESCE(MAX(version_id), 0)
		FROM goose_db_version
		WHERE is_applied = TRUE
	`).Scan(&before)

	if err != nil {
		t.Fatal(err)
	}

	if before != 75 {
		t.Fatalf("expected isolated migration 75, got %d", before)
	}

	cfg := &config.Config{}
	cfg.App.Env = "production"
	cfg.Database.Host = host
	cfg.Database.Port = port
	cfg.Database.User = user
	cfg.Database.Name = name
	cfg.Database.SSLMode = sslmode

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	emptyDir := t.TempDir()

	if err := os.Chdir(emptyDir); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore directory: %v", err)
		}
	}()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := RunMigrations(cfg, log); err != nil {
		t.Fatalf(
			"production startup must verify without running Goose: %v",
			err,
		)
	}

	var after int64

	err = db.QueryRow(`
		SELECT COALESCE(MAX(version_id), 0)
		FROM goose_db_version
		WHERE is_applied = TRUE
	`).Scan(&after)

	if err != nil {
		t.Fatal(err)
	}

	if after != before {
		t.Fatalf(
			"migration history changed: before=%d after=%d",
			before, after,
		)
	}
}
