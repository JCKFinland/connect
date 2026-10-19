package dispatch_offer

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestDispatchOfferAccept(t *testing.T) {
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

	if cfg.Database.Name != "connect_integration_test" {
		t.Fatalf(
			"unsafe integration database: %s",
			cfg.Database.Name,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	// Create an independent customer.
	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated customer: %v", err)
	}

	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup isolated customer: %v", err)
		}
	}()

	// Create an independent company, branch, fleet, driver,
	// vehicle, and driver assignment.
	driverFixture, cleanupDriver, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated driver: %v", err)
	}

	defer func() {
		if err := cleanupDriver(context.Background()); err != nil {
			t.Errorf("cleanup isolated driver: %v", err)
		}
	}()

	// Create a disposable ride request.
	rideRequestID := uuid.NewString()

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
            notes,
            requested_at,
            expires_at,
            created_at,
            updated_at
        )
        VALUES (
            $1, $2,
            'CONNECT isolated offer pickup',
            60.2055, 24.6559,
            'CONNECT isolated offer destination',
            60.1719, 24.9414,
            'STANDARD',
            1,
            'MATCHING',
            'CONNECT isolated dispatch-offer lifecycle test',
            NOW(),
            NOW() + INTERVAL '10 minutes',
            NOW(),
            NOW()
        )
        `,
		rideRequestID,
		customerID,
	)
	if err != nil {
		t.Fatalf("create isolated ride request: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		)
		if err != nil {
			t.Errorf("cleanup isolated ride request: %v", err)
		}
	}()

	repo := postgresrepo.NewDispatchOfferRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Offers:       repo,
			OfferTimeout: 5 * time.Minute,
		},
	)

	// The operational offer driver references drivers.id.
	offer := &models.DispatchOffer{
		RideRequestID: rideRequestID,
		DriverID:      driverFixture.DriverID,
		VehicleID:     driverFixture.VehicleID,
		CompanyID:     driverFixture.CompanyID,
		BranchID:      driverFixture.BranchID,
		FleetID:       driverFixture.FleetID,
		AssignmentID:  &driverFixture.AssignmentID,
	}

	if err := service.Create(ctx, offer); err != nil {
		t.Fatalf("create isolated dispatch offer: %v", err)
	}

	// Remove only this test's offer.
	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM dispatch_offers WHERE id = $1`,
			offer.ID,
		)
		if err != nil {
			t.Errorf("cleanup isolated dispatch offer: %v", err)
		}
	}()

	// ---------------------------------------------------------
	// First acceptance must succeed.
	// ---------------------------------------------------------

	accepted, err := service.Accept(
		ctx,
		offer.ID,
	)
	if err != nil {
		t.Fatalf(
			"accept dispatch offer: %v",
			err,
		)
	}

	if accepted == nil {
		t.Fatal(
			"expected accepted dispatch offer",
		)
	}

	if accepted.Status != StatusAccepted {
		t.Fatalf(
			"expected status %s, got %s",
			StatusAccepted,
			accepted.Status,
		)
	}

	if accepted.RespondedAt == nil {
		t.Fatal(
			"expected responded_at to be populated",
		)
	}

	// ---------------------------------------------------------
	// Verify persisted database state.
	// ---------------------------------------------------------

	persisted, err := service.GetByID(
		ctx,
		offer.ID,
	)
	if err != nil {
		t.Fatalf(
			"get accepted dispatch offer: %v",
			err,
		)
	}

	if persisted.Status != StatusAccepted {
		t.Fatalf(
			"expected persisted status %s, got %s",
			StatusAccepted,
			persisted.Status,
		)
	}

	if persisted.RespondedAt == nil {
		t.Fatal(
			"expected persisted responded_at",
		)
	}

	// ---------------------------------------------------------
	// Second acceptance must fail.
	// ---------------------------------------------------------

	_, err = service.Accept(
		ctx,
		offer.ID,
	)

	if !errors.Is(
		err,
		ErrOfferAlreadyResolved,
	) {
		t.Fatalf(
			"expected ErrOfferAlreadyResolved, got %v",
			err,
		)
	}
}

func TestDispatchOfferReject(t *testing.T) {
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

	if cfg.Database.Name != "connect_integration_test" {
		t.Fatalf(
			"unsafe integration database: %s",
			cfg.Database.Name,
		)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	// Create an independent customer.
	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated customer: %v", err)
	}

	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup isolated customer: %v", err)
		}
	}()

	// Create an independent company, branch, fleet, driver,
	// vehicle, and driver assignment.
	driverFixture, cleanupDriver, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create isolated driver: %v", err)
	}

	defer func() {
		if err := cleanupDriver(context.Background()); err != nil {
			t.Errorf("cleanup isolated driver: %v", err)
		}
	}()

	// Create a disposable ride request.
	rideRequestID := uuid.NewString()

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
            notes,
            requested_at,
            expires_at,
            created_at,
            updated_at
        )
        VALUES (
            $1, $2,
            'CONNECT isolated offer pickup',
            60.2055, 24.6559,
            'CONNECT isolated offer destination',
            60.1719, 24.9414,
            'STANDARD',
            1,
            'MATCHING',
            'CONNECT isolated dispatch-offer lifecycle test',
            NOW(),
            NOW() + INTERVAL '10 minutes',
            NOW(),
            NOW()
        )
        `,
		rideRequestID,
		customerID,
	)
	if err != nil {
		t.Fatalf("create isolated ride request: %v", err)
	}

	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		)
		if err != nil {
			t.Errorf("cleanup isolated ride request: %v", err)
		}
	}()

	repo := postgresrepo.NewDispatchOfferRepository(db)

	service := NewService(
		Dependencies{
			DB:           db,
			Offers:       repo,
			OfferTimeout: 5 * time.Minute,
		},
	)

	// The operational offer driver references drivers.id.
	offer := &models.DispatchOffer{
		RideRequestID: rideRequestID,
		DriverID:      driverFixture.DriverID,
		VehicleID:     driverFixture.VehicleID,
		CompanyID:     driverFixture.CompanyID,
		BranchID:      driverFixture.BranchID,
		FleetID:       driverFixture.FleetID,
		AssignmentID:  &driverFixture.AssignmentID,
	}

	if err := service.Create(ctx, offer); err != nil {
		t.Fatalf("create isolated dispatch offer: %v", err)
	}

	// Remove only this test's offer.
	defer func() {
		_, err := db.Exec(
			context.Background(),
			`DELETE FROM dispatch_offers WHERE id = $1`,
			offer.ID,
		)
		if err != nil {
			t.Errorf("cleanup isolated dispatch offer: %v", err)
		}
	}()

	rejected, err := service.Reject(
		ctx,
		offer.ID,
		"driver unavailable",
	)
	if err != nil {
		t.Fatalf(
			"reject dispatch offer: %v",
			err,
		)
	}

	if rejected.Status != StatusRejected {
		t.Fatalf(
			"expected status %s, got %s",
			StatusRejected,
			rejected.Status,
		)
	}

	if rejected.RespondedAt == nil {
		t.Fatal(
			"expected responded_at to be populated",
		)
	}

	if rejected.RejectionReason == nil ||
		*rejected.RejectionReason != "driver unavailable" {

		t.Fatal(
			"expected rejection reason to be persisted",
		)
	}

	// ---------------------------------------------------------
	// Verify dispatch-offer database state.
	// ---------------------------------------------------------

	persisted, err := service.GetByID(
		ctx,
		offer.ID,
	)
	if err != nil {
		t.Fatalf(
			"get rejected dispatch offer: %v",
			err,
		)
	}

	if persisted.Status != StatusRejected {
		t.Fatalf(
			"expected persisted status %s, got %s",
			StatusRejected,
			persisted.Status,
		)
	}

	// ---------------------------------------------------------
	// Verify ride request MATCHING → PENDING.
	// ---------------------------------------------------------

	var rideRequestStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(&rideRequestStatus); err != nil {
		t.Fatalf(
			"get ride request after rejection: %v",
			err,
		)
	}

	if rideRequestStatus != "PENDING" {
		t.Fatalf(
			"expected ride request status PENDING, got %s",
			rideRequestStatus,
		)
	}

	// ---------------------------------------------------------
	// Second rejection must fail.
	// ---------------------------------------------------------

	_, err = service.Reject(
		ctx,
		offer.ID,
		"second response",
	)

	if !errors.Is(
		err,
		ErrOfferAlreadyResolved,
	) {
		t.Fatalf(
			"expected ErrOfferAlreadyResolved, got %v",
			err,
		)
	}
}

func TestDispatchOfferExpire(t *testing.T) {
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

	releaseFixtureLock, err :=
		testutil.AcquirePostgresFixtureLock(
			ctx,
			db,
			"dispatch-fixture:john",
		)

	if err != nil {
		t.Fatalf(
			"acquire John dispatch fixture lock: %v",
			err,
		)
	}

	defer func() {
		if err := releaseFixtureLock(
			context.Background(),
		); err != nil {
			t.Logf(
				"release John dispatch fixture lock: %v",
				err,
			)
		}
	}()

	repo := postgresrepo.NewDispatchOfferRepository(db)

	expireStaleTestOffers(
		t,
		repo,
	)

	const rideRequestID = "65836dd1-d6e2-4c67-a349-50e081975c78"

	// ---------------------------------------------------------
	// Preserve original ride-request status.
	// ---------------------------------------------------------

	var originalStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(&originalStatus); err != nil {
		t.Fatalf(
			"get original ride request status: %v",
			err,
		)
	}

	defer func() {
		_, restoreErr := db.Exec(
			ctx,
			`
				UPDATE ride_requests
				SET
					status = $2,
					updated_at = NOW()
				WHERE id = $1
			`,
			rideRequestID,
			originalStatus,
		)

		if restoreErr != nil {
			t.Logf(
				"restore ride request status: %v",
				restoreErr,
			)
		}
	}()

	// ---------------------------------------------------------
	// Expiring offer must belong to a MATCHING request.
	// ---------------------------------------------------------

	if _, err := db.Exec(
		ctx,
		`
			UPDATE ride_requests
			SET
				status = 'MATCHING',
				updated_at = NOW()
			WHERE id = $1
		`,
		rideRequestID,
	); err != nil {
		t.Fatalf(
			"prepare MATCHING ride request: %v",
			err,
		)
	}

	service := NewService(
		Dependencies{
			DB:           db,
			Offers:       repo,
			OfferTimeout: 5 * time.Minute,
		},
	)

	now := time.Now().UTC()

	offer := &models.DispatchOffer{
		RideRequestID: rideRequestID,

		DriverID:  "39175f42-0c89-4d45-96be-ed5367506e36",
		VehicleID: "6dce24b5-b257-447a-99e0-ef439fbd0e17",

		CompanyID: "345c5e3e-b07a-4e16-837d-e5d32254d6f3",
		BranchID:  "186f7570-6902-41a2-a1f9-d509a4d90fcb",
		FleetID:   "dc46fc5c-7290-462c-a423-22b3c46b7c99",

		OfferedAt: now.Add(-2 * time.Minute),
		ExpiresAt: now.Add(-1 * time.Minute),
	}

	if err := service.Create(
		ctx,
		offer,
	); err != nil {
		t.Fatalf(
			"create expired dispatch offer: %v",
			err,
		)
	}

	defer func() {
		_, _ = db.Exec(
			ctx,
			`DELETE FROM dispatch_offers WHERE id = $1`,
			offer.ID,
		)
	}()

	expired, err := service.Expire(
		ctx,
		offer.ID,
	)
	if err != nil {
		t.Fatalf(
			"expire dispatch offer: %v",
			err,
		)
	}

	if expired.Status != StatusExpired {
		t.Fatalf(
			"expected status %s, got %s",
			StatusExpired,
			expired.Status,
		)
	}

	if expired.RespondedAt == nil {
		t.Fatal(
			"expected responded_at to be populated",
		)
	}

	// ---------------------------------------------------------
	// Verify persisted offer state.
	// ---------------------------------------------------------

	persisted, err := service.GetByID(
		ctx,
		offer.ID,
	)
	if err != nil {
		t.Fatalf(
			"get expired dispatch offer: %v",
			err,
		)
	}

	if persisted.Status != StatusExpired {
		t.Fatalf(
			"expected persisted status %s, got %s",
			StatusExpired,
			persisted.Status,
		)
	}

	// ---------------------------------------------------------
	// Verify ride request MATCHING → PENDING.
	// ---------------------------------------------------------

	var rideRequestStatus string

	if err := db.QueryRow(
		ctx,
		`
			SELECT status
			FROM ride_requests
			WHERE id = $1
		`,
		rideRequestID,
	).Scan(&rideRequestStatus); err != nil {
		t.Fatalf(
			"get ride request after expiry: %v",
			err,
		)
	}

	if rideRequestStatus != "PENDING" {
		t.Fatalf(
			"expected ride request status PENDING, got %s",
			rideRequestStatus,
		)
	}
}

// expireStaleTestOffers clears time-expired PENDING offers before an
// integration test creates a new offer.
//
// It does not touch valid PENDING offers.
func expireStaleTestOffers(
	t *testing.T,
	repo repository.DispatchOfferRepository,
) {
	t.Helper()

	_, err := repo.ExpireStalePending(
		context.Background(),
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf(
			"expire stale dispatch offers before test: %v",
			err,
		)
	}
}
