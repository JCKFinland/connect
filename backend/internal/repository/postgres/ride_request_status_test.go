package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func TestRideRequestRepositoryUpdateStatusRequiresExpectedCurrentStatus(
	t *testing.T,
) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf(
			"get working directory: %v",
			err,
		)
	}

	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf(
			"change to backend root: %v",
			err,
		)
	}

	defer func() {
		_ = os.Chdir(originalDir)
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf(
			"load CONNECT configuration: %v",
			err,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf(
			"connect database: %v",
			err,
		)
	}
	defer db.Close()

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	rideRequestID := uuid.NewString()
	now := time.Now().UTC()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO ride_requests
			(
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
			VALUES
			(
				$1,
				$2,
				'Status CAS Test Pickup',
				60.2055,
				24.6559,
				'Status CAS Test Destination',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'PENDING',
				'Ride-request status compare-and-transition test',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(15*time.Minute),
	)
	if err != nil {
		t.Fatalf(
			"create status CAS ride request: %v",
			err,
		)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup status CAS ride request: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewRideRequestRepository(db)

	if err := repo.UpdateStatus(
		ctx,
		rideRequestID,
		"PENDING",
		"MATCHING",
	); err != nil {
		t.Fatalf(
			"transition PENDING to MATCHING: %v",
			err,
		)
	}

	err = repo.UpdateStatus(
		ctx,
		rideRequestID,
		"PENDING",
		"CANCELLED",
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound for stale source status, got %v",
			err,
		)
	}

	var persistedStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(&persistedStatus); err != nil {
		t.Fatalf(
			"read status after rejected stale transition: %v",
			err,
		)
	}

	if persistedStatus != "MATCHING" {
		t.Fatalf(
			"expected rejected stale transition to preserve MATCHING, got %s",
			persistedStatus,
		)
	}

	if err := repo.UpdateStatus(
		ctx,
		rideRequestID,
		"MATCHING",
		"CANCELLED",
	); err != nil {
		t.Fatalf(
			"transition MATCHING to CANCELLED: %v",
			err,
		)
	}

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(&persistedStatus); err != nil {
		t.Fatalf(
			"read final ride-request status: %v",
			err,
		)
	}

	if persistedStatus != "CANCELLED" {
		t.Fatalf(
			"expected final status CANCELLED, got %s",
			persistedStatus,
		)
	}
}
