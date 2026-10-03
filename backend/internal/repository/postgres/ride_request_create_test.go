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

func TestRideRequestRepositoryCreateCanonicalizesInitialLifecycle(
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

	if err := repo.Create(ctx, nil); err == nil {
		t.Fatal("expected nil ride request to be rejected")
	}

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	now := time.Now().UTC()
	expiresAt := now.Add(15 * time.Minute)
	poisonedDispatchAt := now.Add(1 * time.Minute)

	request := &models.RideRequest{
		BaseModel: models.BaseModel{
			ID:        uuid.NewString(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		CustomerID:            customerID,
		PickupAddress:         "Creation Authority Pickup",
		PickupLatitude:        60.2055,
		PickupLongitude:       24.6559,
		DestinationAddress:    "Creation Authority Destination",
		DestinationLatitude:   60.1719,
		DestinationLongitude:  24.9414,
		RequestedVehicleType:  "STANDARD",
		PassengerCount:        1,
		Status:                "ACCEPTED",
		Notes:                 "creation lifecycle authority test",
		RequestedAt:           now,
		ExpiresAt:             &expiresAt,
		DispatchRetryCount:    7,
		NextDispatchAttemptAt: &poisonedDispatchAt,
		LastDispatchAttemptAt: &poisonedDispatchAt,
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
			t.Logf("cleanup creation-authority ride request: %v", cleanupErr)
		}
	}()

	if err := repo.Create(ctx, request); err != nil {
		t.Fatalf("create ride request: %v", err)
	}

	if request.Status != "PENDING" {
		t.Fatalf(
			"expected in-memory status PENDING, got %s",
			request.Status,
		)
	}

	if request.DispatchRetryCount != 0 {
		t.Fatalf(
			"expected in-memory dispatch retry count 0, got %d",
			request.DispatchRetryCount,
		)
	}

	if request.NextDispatchAttemptAt != nil {
		t.Fatalf(
			"expected in-memory next dispatch attempt nil, got %v",
			request.NextDispatchAttemptAt,
		)
	}

	if request.LastDispatchAttemptAt != nil {
		t.Fatalf(
			"expected in-memory last dispatch attempt nil, got %v",
			request.LastDispatchAttemptAt,
		)
	}

	var (
		persistedStatus                string
		persistedDispatchRetryCount    int
		persistedNextDispatchAttemptAt *time.Time
		persistedLastDispatchAttemptAt *time.Time
		persistedRequestedAt           time.Time
		persistedExpiresAt             *time.Time
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				status,
				dispatch_retry_count,
				next_dispatch_attempt_at,
				last_dispatch_attempt_at,
				requested_at,
				expires_at
			FROM ride_requests
			WHERE id = $1
		`,
		request.ID,
	).Scan(
		&persistedStatus,
		&persistedDispatchRetryCount,
		&persistedNextDispatchAttemptAt,
		&persistedLastDispatchAttemptAt,
		&persistedRequestedAt,
		&persistedExpiresAt,
	); err != nil {
		t.Fatalf("read persisted ride request: %v", err)
	}

	if persistedStatus != "PENDING" {
		t.Fatalf(
			"expected persisted status PENDING, got %s",
			persistedStatus,
		)
	}

	if persistedDispatchRetryCount != 0 {
		t.Fatalf(
			"expected persisted dispatch retry count 0, got %d",
			persistedDispatchRetryCount,
		)
	}

	if persistedNextDispatchAttemptAt != nil {
		t.Fatalf(
			"expected persisted next dispatch attempt nil, got %v",
			persistedNextDispatchAttemptAt,
		)
	}

	if persistedLastDispatchAttemptAt != nil {
		t.Fatalf(
			"expected persisted last dispatch attempt nil, got %v",
			persistedLastDispatchAttemptAt,
		)
	}

	const timestampTolerance = time.Microsecond

	requestedAtDifference := persistedRequestedAt.Sub(
		request.RequestedAt,
	)
	if requestedAtDifference < 0 {
		requestedAtDifference = -requestedAtDifference
	}

	if requestedAtDifference > timestampTolerance {
		t.Fatalf(
			"expected requested_at within %v of %v, got %v",
			timestampTolerance,
			request.RequestedAt,
			persistedRequestedAt,
		)
	}

	if persistedExpiresAt == nil {
		t.Fatal("expected persisted expires_at")
	}

	expiresAtDifference := persistedExpiresAt.Sub(expiresAt)
	if expiresAtDifference < 0 {
		expiresAtDifference = -expiresAtDifference
	}

	if expiresAtDifference > timestampTolerance {
		t.Fatalf(
			"expected expires_at within %v of %v, got %v",
			timestampTolerance,
			expiresAt,
			persistedExpiresAt,
		)
	}
}
