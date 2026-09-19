package presence

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestGetCurrentReturnsPersistedPresence(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	driverRepo := postgresrepo.NewDriverRepository(db)
	presenceRepo := postgresrepo.NewDriverPresenceRepository(db)

	heartbeatAt := time.Now().UTC().Truncate(time.Microsecond)

	err = presenceRepo.Create(ctx, &models.DriverPresence{
		DriverID:           fixture.UserID,
		CompanyID:          fixture.CompanyID,
		IsOnline:           true,
		AvailabilityStatus: "AVAILABLE",
		LastHeartbeatAt:    &heartbeatAt,
	})
	if err != nil {
		t.Fatalf("create driver presence: %v", err)
	}

	service := NewService(Dependencies{
		Drivers:  driverRepo,
		Presence: presenceRepo,
	})

	result, err := service.GetCurrent(ctx, fixture.UserID)
	if err != nil {
		t.Fatalf("get current presence: %v", err)
	}

	if result.DriverID != fixture.UserID {
		t.Fatalf("expected driver ID %s, got %s", fixture.UserID, result.DriverID)
	}

	if !result.IsOnline {
		t.Fatal("expected driver to be online")
	}

	if result.AvailabilityStatus != "AVAILABLE" {
		t.Fatalf(
			"expected AVAILABLE, got %s",
			result.AvailabilityStatus,
		)
	}

	if result.LastHeartbeatAt == nil {
		t.Fatal("expected last heartbeat")
	}
}

func TestGetCurrentReturnsOfflineWhenPresenceDoesNotExist(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup driver fixture: %v", err)
		}
	}()

	service := NewService(Dependencies{
		Drivers:  postgresrepo.NewDriverRepository(db),
		Presence: postgresrepo.NewDriverPresenceRepository(db),
	})

	result, err := service.GetCurrent(ctx, fixture.UserID)
	if err != nil {
		t.Fatalf("get current presence: %v", err)
	}

	if result.DriverID != fixture.UserID {
		t.Fatalf("expected driver ID %s, got %s", fixture.UserID, result.DriverID)
	}

	if result.IsOnline {
		t.Fatal("expected driver to be offline")
	}

	if result.AvailabilityStatus != "OFFLINE" {
		t.Fatalf("expected OFFLINE, got %s", result.AvailabilityStatus)
	}

	if result.LastHeartbeatAt != nil {
		t.Fatal("expected no heartbeat")
	}
}
