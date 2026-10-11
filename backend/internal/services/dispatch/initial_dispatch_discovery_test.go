package dispatch

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
)

func TestInitialDispatchDiscoversRideWithoutOfferHistory(t *testing.T) {
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

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

	_, err = db.Exec(ctx, `
		INSERT INTO ride_requests (
			id,
			customer_id,
			pickup_address,
			pickup_latitude,
			pickup_longitude,
			destination_address,
			destination_latitude,
			destination_longitude,
			requested_vehicle_type,
			passenger_count,
			status,
			notes,
			requested_at,
			expires_at,
			created_at,
			updated_at
		)
		VALUES (
			$1, $2,
			'Initial Dispatch Test Pickup',
			60.2055, 24.6559,
			'Helsinki Central Station',
			60.1719, 24.9414,
			'STANDARD',
			1,
			'PENDING',
			'Fresh customer booking without offer history',
			$3, $4, $3, $3
		)
	`, rideRequestID, customerID, now, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("create initial dispatch ride request: %v", err)
	}

	defer func() {
		_, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		)
		if cleanupErr != nil {
			t.Logf("cleanup initial dispatch ride: %v", cleanupErr)
		}
	}()

	repo := postgresrepo.NewDispatchOfferRepository(db)

	ids, err := repo.ListRedispatchableRideRequestIDs(ctx, 100)
	if err != nil {
		t.Fatalf("discover initial dispatch candidates: %v", err)
	}

	for _, id := range ids {
		if id == rideRequestID {
			return
		}
	}

	t.Fatalf(
		"fresh PENDING ride %s without offer history was not discovered",
		rideRequestID,
	)
}
