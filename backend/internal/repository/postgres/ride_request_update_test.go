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

func TestRideRequestRepositoryUpdatePreservesLifecycleAuthority(
	t *testing.T,
) {
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

	repo := NewRideRequestRepository(db)

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	now := time.Now().UTC()
	originalExpiresAt := now.Add(15 * time.Minute)

	request := &models.RideRequest{
		BaseModel: models.BaseModel{
			ID:        uuid.NewString(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		CustomerID:           customerID,
		PickupAddress:        "Update Authority Pickup",
		PickupLatitude:       60.2055,
		PickupLongitude:      24.6559,
		DestinationAddress:   "Update Authority Destination",
		DestinationLatitude:  60.1719,
		DestinationLongitude: 24.9414,
		RequestedVehicleType: "STANDARD",
		PassengerCount:       1,
		Status:               "PENDING",
		Notes:                "before generic update",
		RequestedAt:          now,
		ExpiresAt:            &originalExpiresAt,
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			request.ID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup update-authority ride request: %v",
				cleanupErr,
			)
		}
	}()

	if err := repo.Create(ctx, request); err != nil {
		t.Fatalf("create ride request: %v", err)
	}

	poisonedRequestedAt := now.Add(-24 * time.Hour)
	poisonedExpiresAt := now.Add(24 * time.Hour)
	poisonedDispatchAt := now.Add(30 * time.Minute)

	request.Status = "ACCEPTED"
	request.RequestedAt = poisonedRequestedAt
	request.ExpiresAt = &poisonedExpiresAt
	request.DispatchRetryCount = 9
	request.NextDispatchAttemptAt = &poisonedDispatchAt
	request.LastDispatchAttemptAt = &poisonedDispatchAt
	request.Notes = "after generic update"

	if err := repo.Update(ctx, request); err != nil {
		t.Fatalf("update ride request: %v", err)
	}

	var (
		persistedStatus                string
		persistedRequestedAt           time.Time
		persistedExpiresAt             *time.Time
		persistedDispatchRetryCount    int
		persistedNextDispatchAttemptAt *time.Time
		persistedLastDispatchAttemptAt *time.Time
		persistedNotes                 string
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				status,
				requested_at,
				expires_at,
				dispatch_retry_count,
				next_dispatch_attempt_at,
				last_dispatch_attempt_at,
				notes
			FROM ride_requests
			WHERE id = $1
		`,
		request.ID,
	).Scan(
		&persistedStatus,
		&persistedRequestedAt,
		&persistedExpiresAt,
		&persistedDispatchRetryCount,
		&persistedNextDispatchAttemptAt,
		&persistedLastDispatchAttemptAt,
		&persistedNotes,
	); err != nil {
		t.Fatalf("read updated ride request: %v", err)
	}

	if persistedStatus != "PENDING" {
		t.Fatalf(
			"generic update changed protected status: got %s want PENDING",
			persistedStatus,
		)
	}

	const timestampTolerance = time.Microsecond

	requestedAtDifference := persistedRequestedAt.Sub(now)
	if requestedAtDifference < 0 {
		requestedAtDifference = -requestedAtDifference
	}
	if requestedAtDifference > timestampTolerance {
		t.Fatalf(
			"generic update changed protected requested_at: got %v want %v",
			persistedRequestedAt,
			now,
		)
	}

	if persistedExpiresAt == nil {
		t.Fatal("expected protected expires_at to remain set")
	}

	expiresAtDifference := persistedExpiresAt.Sub(originalExpiresAt)
	if expiresAtDifference < 0 {
		expiresAtDifference = -expiresAtDifference
	}
	if expiresAtDifference > timestampTolerance {
		t.Fatalf(
			"generic update changed protected expires_at: got %v want %v",
			persistedExpiresAt,
			originalExpiresAt,
		)
	}

	if persistedDispatchRetryCount != 0 {
		t.Fatalf(
			"generic update changed protected dispatch retry count: got %d want 0",
			persistedDispatchRetryCount,
		)
	}

	if persistedNextDispatchAttemptAt != nil {
		t.Fatalf(
			"generic update changed protected next dispatch attempt: got %v want nil",
			persistedNextDispatchAttemptAt,
		)
	}

	if persistedLastDispatchAttemptAt != nil {
		t.Fatalf(
			"generic update changed protected last dispatch attempt: got %v want nil",
			persistedLastDispatchAttemptAt,
		)
	}

	if persistedNotes != "after generic update" {
		t.Fatalf(
			"generic update did not persist editable notes: got %q",
			persistedNotes,
		)
	}
}
