package database

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestProductionPayoutMigrationGateRejectsNilDatabase(t *testing.T) {
	err := verifyProductionPayoutMigrationGate(nil)

	if err == nil {
		t.Fatal("expected nil database rejection")
	}

	if !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProductionPayoutMigrationGateIsolatedDatabase(t *testing.T) {
	db, err := sql.Open(
		"pgx",
		"host=localhost port=5432 user=postgres "+
			"dbname=connect_payout_destination_test sslmode=disable",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var name string
	var version int64

	err = db.QueryRow(`
		SELECT current_database(),
		       COALESCE(MAX(version_id), 0)
		FROM goose_db_version
		WHERE is_applied = TRUE
	`).Scan(&name, &version)

	if err != nil {
		t.Fatal(err)
	}

	if name != "connect_payout_destination_test" || version != 75 {
		t.Skipf(
			"isolated payout database unavailable: %s version %d",
			name,
			version,
		)
	}

	if err := verifyProductionPayoutMigrationGate(db); err != nil {
		t.Fatalf("expected migration 75 database to pass: %v", err)
	}
}
