package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
)

func TestRideRequestRepositoryMatchingModeReadPaths(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	repo := &RideRequestRepository{db: tx}

	now := time.Now().UTC()
	expiresAt := now.Add(15 * time.Minute)

	request := &models.RideRequest{
		BaseModel: models.BaseModel{
			ID:        uuid.NewString(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		CustomerID:           "49c61249-8b7d-4afd-a559-6d54567ee164",
		PickupAddress:        "Matching Mode Test Pickup",
		PickupLatitude:       60.2055,
		PickupLongitude:      24.6559,
		DestinationAddress:   "Matching Mode Test Destination",
		DestinationLatitude:  60.1719,
		DestinationLongitude: 24.9414,
		RequestedVehicleType: "STANDARD",
		PassengerCount:       1,
		Status:               "PENDING",
		MatchingMode:         models.RideMatchingModeFlex,
		Notes:                "matching mode read-path test",
		RequestedAt:          now,
		ExpiresAt:            &expiresAt,
	}

	if err := repo.Create(ctx, request); err != nil {
		t.Fatalf("create ride request: %v", err)
	}

	assertMode := func(label string, got *models.RideRequest, want string) {
		t.Helper()

		if got == nil {
			t.Fatalf("%s returned nil request", label)
		}
		if got.ID != request.ID {
			t.Fatalf("%s returned ID %s, want %s", label, got.ID, request.ID)
		}
		if got.MatchingMode != want {
			t.Fatalf(
				"%s returned matching mode %s, want %s",
				label,
				got.MatchingMode,
				want,
			)
		}
	}

	standard, err := repo.GetByID(ctx, request.ID)
	if err != nil {
		t.Fatalf("get STANDARD request: %v", err)
	}
	assertMode("GetByID STANDARD", standard, models.RideMatchingModeStandard)

	// Simulate a legitimate matching-mode transition in this isolated
	// transaction. Production activation will have separate authorization
	// and dispatch-history safeguards.
	result, err := tx.Exec(
		ctx,
		`UPDATE ride_requests
		 SET matching_mode = $1
		 WHERE id = $2
		   AND matching_mode = $3
		   AND status = 'PENDING'`,
		models.RideMatchingModeFlex,
		request.ID,
		models.RideMatchingModeStandard,
	)
	if err != nil {
		t.Fatalf("set test request to FLEX: %v", err)
	}
	if result.RowsAffected() != 1 {
		t.Fatalf("expected one FLEX transition, got %d", result.RowsAffected())
	}

	byID, err := repo.GetByID(ctx, request.ID)
	if err != nil {
		t.Fatalf("get FLEX request: %v", err)
	}
	assertMode("GetByID FLEX", byID, models.RideMatchingModeFlex)

	listed, err := repo.List(ctx, request.CustomerID, "PENDING", 100, 0)
	if err != nil {
		t.Fatalf("list FLEX requests: %v", err)
	}

	found := false
	for _, item := range listed {
		if item.ID == request.ID {
			assertMode("List FLEX", item, models.RideMatchingModeFlex)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("List did not return the test ride request")
	}

	locked, err := repo.GetByIDForUpdate(ctx, request.ID)
	if err != nil {
		t.Fatalf("get FLEX request FOR UPDATE: %v", err)
	}
	assertMode(
		"GetByIDForUpdate FLEX",
		locked,
		models.RideMatchingModeFlex,
	)

	// Generic updates must not change workflow ownership.
	locked.MatchingMode = models.RideMatchingModeStandard
	locked.Notes = "updated without changing matching mode"

	if err := repo.Update(ctx, locked); err != nil {
		t.Fatalf("generic ride-request update: %v", err)
	}

	afterUpdate, err := repo.GetByID(ctx, request.ID)
	if err != nil {
		t.Fatalf("get request after generic update: %v", err)
	}
	assertMode(
		"GetByID after Update",
		afterUpdate,
		models.RideMatchingModeFlex,
	)

	if afterUpdate.Notes != locked.Notes {
		t.Fatalf(
			"generic update did not persist notes: got %q, want %q",
			afterUpdate.Notes,
			locked.Notes,
		)
	}

	// Rollback removes the fixture and every test-only transition.
}
