package database

import (
	"os"
	"strings"
	"testing"
)

func TestProductionMigrationRunnerDoesNotAutomaticallyApplyMigrations(t *testing.T) {
	source, err := os.ReadFile("migrations.go")
	if err != nil {
		t.Fatal(err)
	}

	text := string(source)

	production := strings.Index(
		text,
		`if cfg.App.Env == "production" {`,
	)

	gooseUp := strings.Index(
		text,
		`goose.Up(db, migrationsDir)`,
	)

	if production < 0 || gooseUp < 0 {
		t.Fatal("expected production guard and Goose migration call")
	}

	section := text[production:gooseUp]

	if !strings.Contains(section, "verifyProductionPayoutMigrationGate(db)") {
		t.Fatal("production startup must verify payout schema")
	}

	if !strings.Contains(section, "return nil") {
		t.Fatal("production startup must return before automatic migrations")
	}
}
