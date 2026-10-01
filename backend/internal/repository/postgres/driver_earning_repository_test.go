package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestDriverEarningRepositoryCreateFromPaidPayment(t *testing.T) {
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
		t.Fatalf("acquire shared fixture lock: %v", err)
	}

	defer func() {
		if err := releaseFixtureLock(context.Background()); err != nil {
			t.Logf("release shared fixture lock: %v", err)
		}
	}()

	const customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"

	driverFixture, cleanupDriverFixture, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf("create isolated driver fixture: %v", err)
	}

	defer func() {
		if err := cleanupDriverFixture(context.Background()); err != nil {
			t.Logf("cleanup isolated driver fixture: %v", err)
		}
	}()

	driverID := driverFixture.UserID
	companyID := driverFixture.CompanyID
	branchID := driverFixture.BranchID
	fleetID := driverFixture.FleetID
	vehicleID := driverFixture.VehicleID

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()
	fareID := uuid.NewString()

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
				'Driver Earning Test Pickup',
				60.2055,
				24.6559,
				'Driver Earning Test Destination',
				60.1719,
				24.9414,
				'STANDARD',
				1,
				'ACCEPTED',
				$3,
				$4,
				$3,
				$3
			)
		`,
		rideRequestID,
		customerID,
		now,
		now.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatalf("create driver earning test ride request: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf("cleanup ride request: %v", cleanupErr)
		}
	}()
	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trips (
				id,
				ride_request_id,
				customer_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				status,
				is_active,
				actual_distance_meters,
				actual_duration_seconds,
				assigned_at,
				started_at,
				completed_at,
				created_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				'COMPLETED',
				FALSE,
				8420,
				900,
				$9,
				$10,
				$11,
				$9,
				$11
			)
		`,
		tripID,
		rideRequestID,
		customerID,
		driverID,
		vehicleID,
		companyID,
		branchID,
		fleetID,
		now.Add(-20*time.Minute),
		now.Add(-15*time.Minute),
		now,
	)
	if err != nil {
		t.Fatalf("create driver earning test trip: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf("cleanup trip: %v", cleanupErr)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trip_fares (
				id,
				trip_id,
				base_fare,
				distance_fare,
				time_fare,
				waiting_fare,
				booking_fee,
				surge_multiplier,
				surge_amount,
				discount_amount,
				tax_amount,
				toll_amount,
				parking_amount,
				total_amount,
				currency,
				distance_rate_per_km,
				time_rate_per_minute,
				waiting_rate_per_minute,
				charged_distance_meters,
				charged_duration_seconds,
				waiting_duration_seconds,
				pricing_version,
				calculated_at,
				created_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				4.90,
				12.63,
				3.75,
				1.25,
				2.00,
				1.00,
				0,
				0,
				5.17,
				0,
				0,
				29.70,
				'EUR',
				1.5000,
				0.2500,
				0.2500,
				8420,
				900,
				300,
				'driver-earning-test-v1',
				$3,
				$3,
				$3
			)
		`,
		fareID,
		tripID,
		now,
	)
	if err != nil {
		t.Fatalf("create driver earning test fare: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM trip_fares WHERE id = $1`,
			fareID,
		); cleanupErr != nil {
			t.Logf("cleanup trip fare: %v", cleanupErr)
		}
	}()

	paymentRepo := NewPaymentRepository(db)

	payment, err := paymentRepo.CreateFromCompletedTrip(
		ctx,
		tripID,
		"CARD",
	)
	if err != nil {
		t.Fatalf("create driver earning test payment: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM payments WHERE id = $1`,
			payment.ID,
		); cleanupErr != nil {
			t.Logf("cleanup payment: %v", cleanupErr)
		}
	}()

	if err := paymentRepo.UpdateStatus(
		ctx,
		payment.ID,
		"PAID",
	); err != nil {
		t.Fatalf("mark driver earning test payment paid: %v", err)
	}

	paidPayment, err := paymentRepo.GetByID(
		ctx,
		payment.ID,
	)
	if err != nil {
		t.Fatalf("reload paid payment: %v", err)
	}

	if paidPayment.Status != "PAID" {
		t.Fatalf(
			"payment status mismatch: got %s want PAID",
			paidPayment.Status,
		)
	}

	if paidPayment.Amount != "29.70" {
		t.Fatalf(
			"payment amount mismatch: got %s want 29.70",
			paidPayment.Amount,
		)
	}

	earningRepo := NewDriverEarningRepository(db)

	earning, err := earningRepo.CreateFromPaidPayment(
		ctx,
		payment.ID,
	)
	if err != nil {
		t.Fatalf("create driver earning from paid payment: %v", err)
	}

	if earning.ID == "" {
		t.Fatal("expected driver earning ID")
	}

	if earning.TripID != tripID {
		t.Fatalf(
			"earning trip ID mismatch: got %s want %s",
			earning.TripID,
			tripID,
		)
	}

	if earning.DriverID != driverID {
		t.Fatalf(
			"earning driver ID mismatch: got %s want %s",
			earning.DriverID,
			driverID,
		)
	}

	if earning.CompanyID != companyID {
		t.Fatalf(
			"earning company ID mismatch: got %s want %s",
			earning.CompanyID,
			companyID,
		)
	}

	if earning.FareID != fareID {
		t.Fatalf(
			"earning fare ID mismatch: got %s want %s",
			earning.FareID,
			fareID,
		)
	}

	if earning.PaymentID == nil {
		t.Fatal("expected earning payment ID")
	}

	if *earning.PaymentID != payment.ID {
		t.Fatalf(
			"earning payment ID mismatch: got %s want %s",
			*earning.PaymentID,
			payment.ID,
		)
	}

	if earning.GrossAmount != "29.70" {
		t.Fatalf(
			"gross amount mismatch: got %s want 29.70",
			earning.GrossAmount,
		)
	}

	if earning.CommissionAmount != "0.00" {
		t.Fatalf(
			"commission amount mismatch: got %s want 0.00",
			earning.CommissionAmount,
		)
	}

	if earning.BonusAmount != "0.00" {
		t.Fatalf(
			"bonus amount mismatch: got %s want 0.00",
			earning.BonusAmount,
		)
	}

	if earning.TipAmount != "0.00" {
		t.Fatalf(
			"tip amount mismatch: got %s want 0.00",
			earning.TipAmount,
		)
	}

	if earning.AdjustmentAmount != "0.00" {
		t.Fatalf(
			"adjustment amount mismatch: got %s want 0.00",
			earning.AdjustmentAmount,
		)
	}

	if earning.TaxWithheld != "0.00" {
		t.Fatalf(
			"tax withheld mismatch: got %s want 0.00",
			earning.TaxWithheld,
		)
	}

	if earning.NetAmount != "29.70" {
		t.Fatalf(
			"net amount mismatch: got %s want 29.70",
			earning.NetAmount,
		)
	}

	if earning.Currency != "EUR" {
		t.Fatalf(
			"currency mismatch: got %s want EUR",
			earning.Currency,
		)
	}

	if earning.SettlementStatus != "PENDING" {
		t.Fatalf(
			"settlement status mismatch: got %s want PENDING",
			earning.SettlementStatus,
		)
	}

	byTripID, err := earningRepo.GetByTripID(
		ctx,
		tripID,
	)
	if err != nil {
		t.Fatalf("get driver earning by trip ID: %v", err)
	}

	if byTripID.ID != earning.ID {
		t.Fatalf(
			"retrieved earning ID mismatch: got %s want %s",
			byTripID.ID,
			earning.ID,
		)
	}

	replayed, err := earningRepo.CreateFromPaidPayment(
		ctx,
		payment.ID,
	)
	if err != nil {
		t.Fatalf("replay driver earning creation: %v", err)
	}

	summary, err :=
		earningRepo.GetSummaryByDriverID(
			ctx,
			driverFixture.UserID,
		)
	if err != nil {
		t.Fatalf(
			"get driver earnings summary: %v",
			err,
		)
	}

	if summary.GrossAmount != "29.70" {
		t.Fatalf(
			"summary gross amount mismatch: got %s want 29.70",
			summary.GrossAmount,
		)
	}

	if summary.NetAmount != "29.70" {
		t.Fatalf(
			"summary net amount mismatch: got %s want 29.70",
			summary.NetAmount,
		)
	}

	if summary.Currency != "EUR" {
		t.Fatalf(
			"summary currency mismatch: got %s want EUR",
			summary.Currency,
		)
	}

	if summary.TripCount != 1 {
		t.Fatalf(
			"summary trip count mismatch: got %d want 1",
			summary.TripCount,
		)
	}

	recent, err :=
		earningRepo.ListRecentByDriverID(
			ctx,
			driverFixture.UserID,
			50,
		)
	if err != nil {
		t.Fatalf(
			"list recent driver earnings: %v",
			err,
		)
	}

	if len(recent) != 1 {
		t.Fatalf(
			"recent earnings count mismatch: got %d want 1",
			len(recent),
		)
	}

	if recent[0].ID != earning.ID {
		t.Fatalf(
			"recent earning ID mismatch: got %s want %s",
			recent[0].ID,
			earning.ID,
		)
	}

	if replayed.ID != earning.ID {
		t.Fatalf(
			"replayed earning ID mismatch: got %s want %s",
			replayed.ID,
			earning.ID,
		)
	}

	var earningCount int

	if err := db.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM driver_earnings WHERE trip_id = $1`,
		tripID,
	).Scan(&earningCount); err != nil {
		t.Fatalf("count driver earnings for trip: %v", err)
	}

	if earningCount != 1 {
		t.Fatalf(
			"expected exactly 1 earning for trip, got %d",
			earningCount,
		)
	}
}
