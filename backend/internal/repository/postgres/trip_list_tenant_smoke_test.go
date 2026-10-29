package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
)

func TestTenantListSQLSmokeIsolated(t *testing.T) {
	const safeDatabase = "connect_integration_test"

	if os.Getenv("DB_NAME") != safeDatabase {
		t.Fatalf(
			"refusing unsafe database: %q",
			os.Getenv("DB_NAME"),
		)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	backendRoot := filepath.Clean(
		filepath.Join(originalDir, "../../.."),
	)

	if _, err := os.Stat(
		filepath.Join(backendRoot, "go.mod"),
	); err != nil {
		t.Fatalf("backend root not found: %v", err)
	}

	if _, err := os.Stat(
		filepath.Join(backendRoot, ".env"),
	); err != nil {
		t.Fatalf("backend .env not found: %v", err)
	}

	if err := os.Chdir(backendRoot); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}

	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	if cfg.Database.Name != safeDatabase {
		t.Fatalf(
			"unsafe configured database: %q",
			cfg.Database.Name,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect isolated database: %v",
			err,
		)
	}
	defer db.Close()

	repo := NewTripRepository(db)

	trips, err := repo.ListForCompanyMember(
		context.Background(),
		uuid.NewString(),
		"",
		"",
		"",
		"",
		"",
		10,
		0,
	)
	if err != nil {
		t.Fatalf(
			"execute tenant-scoped SQL: %v",
			err,
		)
	}

	if len(trips) != 0 {
		t.Fatalf(
			"expected no trips for user without membership; got %d",
			len(trips),
		)
	}
}
