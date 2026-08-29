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

func TestTripFareRepositoryRoundTrip(t *testing.T) {
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

	// ---------------------------------------------------------
	// Serialize access to John's shared integration fixture.
	//
	// Other integration tests temporarily modify John's active
	// assignment, vehicle, or presence. Without the advisory lock,
	// this test can race with them when go test ./... runs packages
	// concurrently.
	// ---------------------------------------------------------

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

	const (
		customerID = "49c61249-8b7d-4afd-a559-6d54567ee164"
		driverID   = "ba7cead1-34a0-4df1-ade4-145441ee8559"
	)

	var (
		companyID string
		branchID  string
		fleetID   string
		vehicleID string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				company_id,
				branch_id,
				fleet_id,
				vehicle_id
			FROM driver_assignments
			WHERE driver_id = $1
			  AND unassigned_at IS NULL
			LIMIT 1
		`,
		driverID,
	).Scan(
		&companyID,
		&branchID,
		&fleetID,
		&vehicleID,
	)
	if err != nil {
		t.Fatalf(
			"resolve active driver assignment: %v",
			err,
		)
	}

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

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
				requested_at,
				expires_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				'Trip Fare Repository Test Pickup',
				60.2055,
				24.6559,
				'Trip Fare Repository Test Destination',
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
		t.Fatalf(
			"create fare test ride request: %v",
			err,
		)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO trips
			(
				id,
				ride_request_id,
				customer_id,
				driver_id,
				vehicle_id,
				company_id,
				branch_id,
				fleet_id,
				status,
				actual_distance_meters,
				actual_duration_seconds,
				assigned_at,
				started_at,
				completed_at,
				created_at,
				updated_at
			)
			VALUES
			(
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				'COMPLETED',
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
		t.Fatalf(
			"create fare test trip: %v",
			err,
		)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM trips
				WHERE id = $1
			`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup fare test trip: %v",
				cleanupErr,
			)
		}

		if _, cleanupErr := db.Exec(
			context.Background(),
			`
				DELETE FROM ride_requests
				WHERE id = $1
			`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup fare test ride request: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewTripFareRepository(db)

	calculatedAt := time.Now().UTC()

	fare := &models.TripFare{
		TripID: tripID,

		BaseFare:     4.90,
		DistanceFare: 12.63,
		TimeFare:     3.75,
		WaitingFare:  1.25,
		BookingFee:   2.00,

		SurgeMultiplier: 1.00,
		SurgeAmount:     0,

		DiscountAmount: 0,
		TaxAmount:      5.17,
		TollAmount:     0,
		ParkingAmount:  0,

		TotalAmount: 29.70,
		Currency:    "EUR",

		DistanceRatePerKM:    1.5000,
		TimeRatePerMinute:    0.2500,
		WaitingRatePerMinute: 0.5000,

		ChargedDistanceMeters:  8420,
		ChargedDurationSeconds: 900,
		WaitingDurationSeconds: 150,

		PricingVersion: "v1",

		CalculatedAt: calculatedAt,
	}

	if err := repo.Create(ctx, fare); err != nil {
		t.Fatalf(
			"create trip fare: %v",
			err,
		)
	}

	if fare.ID == "" {
		t.Fatal(
			"expected created fare ID",
		)
	}

	if fare.CreatedAt.IsZero() {
		t.Fatal(
			"expected created_at to be populated",
		)
	}

	if fare.UpdatedAt.IsZero() {
		t.Fatal(
			"expected updated_at to be populated",
		)
	}

	byID, err := repo.GetByID(
		ctx,
		fare.ID,
	)
	if err != nil {
		t.Fatalf(
			"get trip fare by ID: %v",
			err,
		)
	}

	assertTripFareMatches(
		t,
		fare,
		byID,
	)

	byTripID, err := repo.GetByTripID(
		ctx,
		tripID,
	)
	if err != nil {
		t.Fatalf(
			"get trip fare by trip ID: %v",
			err,
		)
	}

	assertTripFareMatches(
		t,
		fare,
		byTripID,
	)

	_, err = repo.GetByID(
		ctx,
		uuid.NewString(),
	)
	if !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatalf(
			"expected ErrNotFound for unknown fare ID, got %v",
			err,
		)
	}

	_, err = repo.GetByTripID(
		ctx,
		uuid.NewString(),
	)
	if !errors.Is(
		err,
		repository.ErrNotFound,
	) {
		t.Fatalf(
			"expected ErrNotFound for unknown trip ID, got %v",
			err,
		)
	}

	duplicateFare := &models.TripFare{
		TripID: tripID,

		SurgeMultiplier: 1.00,
		TotalAmount:     10.00,
		Currency:        "EUR",

		PricingVersion: "v1",
		CalculatedAt:   time.Now().UTC(),
	}

	err = repo.Create(
		ctx,
		duplicateFare,
	)
	if err == nil {
		t.Fatal(
			"expected duplicate fare for same trip to be rejected",
		)
	}
}

func assertTripFareMatches(
	t *testing.T,
	expected *models.TripFare,
	actual *models.TripFare,
) {
	t.Helper()

	if actual == nil {
		t.Fatal(
			"expected trip fare, got nil",
		)
	}

	if actual.ID != expected.ID {
		t.Fatalf(
			"expected fare ID %s, got %s",
			expected.ID,
			actual.ID,
		)
	}

	if actual.TripID != expected.TripID {
		t.Fatalf(
			"expected trip ID %s, got %s",
			expected.TripID,
			actual.TripID,
		)
	}

	if actual.BaseFare != expected.BaseFare {
		t.Fatalf(
			"expected base fare %.2f, got %.2f",
			expected.BaseFare,
			actual.BaseFare,
		)
	}

	if actual.DistanceFare != expected.DistanceFare {
		t.Fatalf(
			"expected distance fare %.2f, got %.2f",
			expected.DistanceFare,
			actual.DistanceFare,
		)
	}

	if actual.TimeFare != expected.TimeFare {
		t.Fatalf(
			"expected time fare %.2f, got %.2f",
			expected.TimeFare,
			actual.TimeFare,
		)
	}

	if actual.WaitingFare != expected.WaitingFare {
		t.Fatalf(
			"expected waiting fare %.2f, got %.2f",
			expected.WaitingFare,
			actual.WaitingFare,
		)
	}

	if actual.BookingFee != expected.BookingFee {
		t.Fatalf(
			"expected booking fee %.2f, got %.2f",
			expected.BookingFee,
			actual.BookingFee,
		)
	}

	if actual.SurgeMultiplier != expected.SurgeMultiplier {
		t.Fatalf(
			"expected surge multiplier %.2f, got %.2f",
			expected.SurgeMultiplier,
			actual.SurgeMultiplier,
		)
	}

	if actual.SurgeAmount != expected.SurgeAmount {
		t.Fatalf(
			"expected surge amount %.2f, got %.2f",
			expected.SurgeAmount,
			actual.SurgeAmount,
		)
	}

	if actual.DiscountAmount != expected.DiscountAmount {
		t.Fatalf(
			"expected discount amount %.2f, got %.2f",
			expected.DiscountAmount,
			actual.DiscountAmount,
		)
	}

	if actual.TaxAmount != expected.TaxAmount {
		t.Fatalf(
			"expected tax amount %.2f, got %.2f",
			expected.TaxAmount,
			actual.TaxAmount,
		)
	}

	if actual.TollAmount != expected.TollAmount {
		t.Fatalf(
			"expected toll amount %.2f, got %.2f",
			expected.TollAmount,
			actual.TollAmount,
		)
	}

	if actual.ParkingAmount != expected.ParkingAmount {
		t.Fatalf(
			"expected parking amount %.2f, got %.2f",
			expected.ParkingAmount,
			actual.ParkingAmount,
		)
	}

	if actual.TotalAmount != expected.TotalAmount {
		t.Fatalf(
			"expected total amount %.2f, got %.2f",
			expected.TotalAmount,
			actual.TotalAmount,
		)
	}

	if actual.Currency != expected.Currency {
		t.Fatalf(
			"expected currency %s, got %s",
			expected.Currency,
			actual.Currency,
		)
	}

	if actual.DistanceRatePerKM != expected.DistanceRatePerKM {
		t.Fatalf(
			"expected distance rate %.4f, got %.4f",
			expected.DistanceRatePerKM,
			actual.DistanceRatePerKM,
		)
	}

	if actual.TimeRatePerMinute != expected.TimeRatePerMinute {
		t.Fatalf(
			"expected time rate %.4f, got %.4f",
			expected.TimeRatePerMinute,
			actual.TimeRatePerMinute,
		)
	}

	if actual.WaitingRatePerMinute != expected.WaitingRatePerMinute {
		t.Fatalf(
			"expected waiting rate %.4f, got %.4f",
			expected.WaitingRatePerMinute,
			actual.WaitingRatePerMinute,
		)
	}

	if actual.ChargedDistanceMeters != expected.ChargedDistanceMeters {
		t.Fatalf(
			"expected charged distance %d, got %d",
			expected.ChargedDistanceMeters,
			actual.ChargedDistanceMeters,
		)
	}

	if actual.ChargedDurationSeconds != expected.ChargedDurationSeconds {
		t.Fatalf(
			"expected charged duration %d, got %d",
			expected.ChargedDurationSeconds,
			actual.ChargedDurationSeconds,
		)
	}

	if actual.WaitingDurationSeconds != expected.WaitingDurationSeconds {
		t.Fatalf(
			"expected waiting duration %d, got %d",
			expected.WaitingDurationSeconds,
			actual.WaitingDurationSeconds,
		)
	}

	if actual.PricingVersion != expected.PricingVersion {
		t.Fatalf(
			"expected pricing version %s, got %s",
			expected.PricingVersion,
			actual.PricingVersion,
		)
	}

	const timestampTolerance = time.Millisecond

	calculatedDifference :=
		actual.CalculatedAt.Sub(
			expected.CalculatedAt,
		)

	if calculatedDifference < 0 {
		calculatedDifference =
			-calculatedDifference
	}

	if calculatedDifference >
		timestampTolerance {
		t.Fatalf(
			"expected calculated_at about %v, got %v",
			expected.CalculatedAt,
			actual.CalculatedAt,
		)
	}
}
