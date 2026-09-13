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
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestDispatchOfferRepositoryGetPendingByDriverExcludesExpiredOffer(
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

	driverFixture, cleanupDriverFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf(
			"create isolated driver fixture: %v",
			err,
		)
	}

	defer func() {
		if err := cleanupDriverFixture(
			context.Background(),
		); err != nil {
			t.Logf(
				"cleanup isolated driver fixture: %v",
				err,
			)
		}
	}()

	rideRequestID := uuid.NewString()
	offerID := uuid.NewString()
	now := time.Now().UTC()

	_, err = db.Exec(
		ctx,
		`
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
				requested_at,
				expires_at,
				created_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				'Expired Offer Test Pickup',
				60.2055,
				24.6559,
				'Expired Offer Test Destination',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'MATCHING',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		driverFixture.UserID,
		now.Add(-10*time.Minute),
		now.Add(20*time.Minute),
	)
	if err != nil {
		t.Fatalf(
			"create expired-offer ride request: %v",
			err,
		)
	}

	defer func() {
		cleanupCtx := context.Background()

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM dispatch_offers WHERE id = $1`,
			offerID,
		); err != nil {
			t.Logf(
				"cleanup expired dispatch offer: %v",
				err,
			)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); err != nil {
			t.Logf(
				"cleanup expired-offer ride request: %v",
				err,
			)
		}
	}()

	repo := NewDispatchOfferRepository(db)
	createdBy := driverFixture.UserID

	offer := &models.DispatchOffer{
		ID:            offerID,
		RideRequestID: rideRequestID,
		DriverID:      driverFixture.DriverID,
		VehicleID:     driverFixture.VehicleID,
		CompanyID:     driverFixture.CompanyID,
		BranchID:      driverFixture.BranchID,
		FleetID:       driverFixture.FleetID,
		Status:        "PENDING",
		OfferedAt:     now.Add(-2 * time.Minute),
		ExpiresAt:     now.Add(-1 * time.Minute),
		CreatedBy:     &createdBy,
		CreatedAt:     now.Add(-2 * time.Minute),
		UpdatedAt:     now.Add(-2 * time.Minute),
	}

	if err := repo.Create(ctx, offer); err != nil {
		t.Fatalf(
			"create expired pending dispatch offer: %v",
			err,
		)
	}

	got, err := repo.GetPendingByDriver(
		ctx,
		driverFixture.DriverID,
	)

	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound for expired pending offer, got offer=%v err=%v",
			got,
			err,
		)
	}

	if got != nil {
		t.Fatalf(
			"expected expired pending offer to be excluded, got %+v",
			got,
		)
	}
}
