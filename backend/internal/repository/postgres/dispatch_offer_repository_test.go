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

func TestDispatchOfferRepositoryUpdateStatusOnlyResolvesPendingOnce(
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
                                'Offer Status Authority Pickup',
                                60.2055,
                                24.6559,
                                'Offer Status Authority Destination',
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
		now,
		now.Add(20*time.Minute),
	)
	if err != nil {
		t.Fatalf(
			"create status-authority ride request: %v",
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
				"cleanup status-authority dispatch offer: %v",
				err,
			)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); err != nil {
			t.Logf(
				"cleanup status-authority ride request: %v",
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
		OfferedAt:     now,
		ExpiresAt:     now.Add(2 * time.Minute),
		CreatedBy:     &createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := repo.Create(ctx, offer); err != nil {
		t.Fatalf(
			"create pending dispatch offer: %v",
			err,
		)
	}

	acceptedAt := now.Add(30 * time.Second)

	if err := repo.UpdateStatus(
		ctx,
		offerID,
		"ACCEPTED",
		&acceptedAt,
		nil,
	); err != nil {
		t.Fatalf(
			"resolve PENDING offer as ACCEPTED: %v",
			err,
		)
	}

	rejectedAt := now.Add(45 * time.Second)
	rejectionReason := "stale rewrite must fail"

	err = repo.UpdateStatus(
		ctx,
		offerID,
		"REJECTED",
		&rejectedAt,
		&rejectionReason,
	)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound when rewriting resolved offer, got %v",
			err,
		)
	}

	var (
		persistedStatus          string
		persistedRespondedAt     *time.Time
		persistedRejectionReason *string
	)

	if err := db.QueryRow(
		ctx,
		`
                        SELECT
                                status,
                                responded_at,
                                rejection_reason
                        FROM dispatch_offers
                        WHERE id = $1
                `,
		offerID,
	).Scan(
		&persistedStatus,
		&persistedRespondedAt,
		&persistedRejectionReason,
	); err != nil {
		t.Fatalf(
			"read resolved dispatch offer: %v",
			err,
		)
	}

	if persistedStatus != "ACCEPTED" {
		t.Fatalf(
			"stale rewrite changed resolved status: got %s want ACCEPTED",
			persistedStatus,
		)
	}

	if persistedRespondedAt == nil {
		t.Fatal("expected accepted responded_at to remain set")
	}

	const timestampTolerance = time.Microsecond

	respondedAtDifference :=
		persistedRespondedAt.Sub(acceptedAt)
	if respondedAtDifference < 0 {
		respondedAtDifference = -respondedAtDifference
	}

	if respondedAtDifference > timestampTolerance {
		t.Fatalf(
			"stale rewrite changed responded_at: got %v want %v",
			persistedRespondedAt,
			acceptedAt,
		)
	}

	if persistedRejectionReason != nil {
		t.Fatalf(
			"stale rewrite changed rejection reason: got %q want nil",
			*persistedRejectionReason,
		)
	}
}

func TestDispatchOfferRepositoryCreateCanonicalizesInitialLifecycle(
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

	repo := NewDispatchOfferRepository(db)

	if err := repo.Create(ctx, nil); err == nil {
		t.Fatal("expected nil dispatch offer to be rejected")
	}

	driverFixture, cleanupDriverFixture, err :=
		testutil.CreateDriverFixture(ctx, db)
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
	offeredAt := now.Add(time.Minute)
	expiresAt := offeredAt.Add(2 * time.Minute)
	poisonedRespondedAt :=
		offeredAt.Add(30 * time.Second)

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
				'Offer Creation Authority Pickup',
				60.2055,
				24.6559,
				'Offer Creation Authority Destination',
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
		now,
		now.Add(20*time.Minute),
	)
	if err != nil {
		t.Fatalf(
			"create creation-authority ride request: %v",
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
				"cleanup creation-authority dispatch offer: %v",
				err,
			)
		}

		if _, err := db.Exec(
			cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); err != nil {
			t.Logf(
				"cleanup creation-authority ride request: %v",
				err,
			)
		}
	}()

	createdBy := driverFixture.UserID
	rejectionReason := "poisoned creation evidence"

	offer := &models.DispatchOffer{
		ID:              offerID,
		RideRequestID:   rideRequestID,
		DriverID:        driverFixture.DriverID,
		VehicleID:       driverFixture.VehicleID,
		CompanyID:       driverFixture.CompanyID,
		BranchID:        driverFixture.BranchID,
		FleetID:         driverFixture.FleetID,
		Status:          "REJECTED",
		OfferedAt:       offeredAt,
		ExpiresAt:       expiresAt,
		RespondedAt:     &poisonedRespondedAt,
		RejectionReason: &rejectionReason,
		CreatedBy:       &createdBy,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := repo.Create(ctx, offer); err != nil {
		t.Fatalf("create dispatch offer: %v", err)
	}

	if offer.Status != "PENDING" {
		t.Fatalf(
			"expected in-memory status PENDING, got %s",
			offer.Status,
		)
	}

	if offer.RespondedAt != nil {
		t.Fatalf(
			"expected in-memory responded_at nil, got %v",
			offer.RespondedAt,
		)
	}

	if offer.RejectionReason != nil {
		t.Fatalf(
			"expected in-memory rejection reason nil, got %q",
			*offer.RejectionReason,
		)
	}

	var (
		persistedStatus          string
		persistedOfferedAt       time.Time
		persistedExpiresAt       time.Time
		persistedRespondedAt     *time.Time
		persistedRejectionReason *string
		persistedCreatedBy       *string
	)

	if err := db.QueryRow(
		ctx,
		`
			SELECT
				status,
				offered_at,
				expires_at,
				responded_at,
				rejection_reason,
				created_by
			FROM dispatch_offers
			WHERE id = $1
		`,
		offerID,
	).Scan(
		&persistedStatus,
		&persistedOfferedAt,
		&persistedExpiresAt,
		&persistedRespondedAt,
		&persistedRejectionReason,
		&persistedCreatedBy,
	); err != nil {
		t.Fatalf(
			"read creation-authority dispatch offer: %v",
			err,
		)
	}

	if persistedStatus != "PENDING" {
		t.Fatalf(
			"expected persisted status PENDING, got %s",
			persistedStatus,
		)
	}

	if persistedRespondedAt != nil {
		t.Fatalf(
			"expected persisted responded_at nil, got %v",
			persistedRespondedAt,
		)
	}

	if persistedRejectionReason != nil {
		t.Fatalf(
			"expected persisted rejection reason nil, got %q",
			*persistedRejectionReason,
		)
	}

	if !persistedOfferedAt.Equal(
		offeredAt.Truncate(time.Microsecond),
	) {
		t.Fatalf(
			"offered_at changed: got %v want %v",
			persistedOfferedAt,
			offeredAt,
		)
	}

	if !persistedExpiresAt.Equal(
		expiresAt.Truncate(time.Microsecond),
	) {
		t.Fatalf(
			"expires_at changed: got %v want %v",
			persistedExpiresAt,
			expiresAt,
		)
	}

	if persistedCreatedBy == nil ||
		*persistedCreatedBy != createdBy {
		t.Fatalf(
			"created_by changed: got %v want %s",
			persistedCreatedBy,
			createdBy,
		)
	}
}
