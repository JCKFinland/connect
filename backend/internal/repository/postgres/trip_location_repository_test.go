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
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestTripLocationRepositoryCreateAndListByTripID(t *testing.T) {
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

	// Serialize access to John's shared integration fixture.
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

	now := time.Now().UTC().Truncate(time.Microsecond)

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
				'Trip Location Repository Test Pickup',
				60.169856,
				24.938379,
				'Trip Location Repository Test Destination',
				60.170500,
				24.940000,
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
			"create trip location test ride request: %v",
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
				assigned_at,
				started_at,
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
				'IN_PROGRESS',
				$9,
				$10,
				$9,
				$10
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
		now.Add(-10*time.Minute),
		now.Add(-5*time.Minute),
	)
	if err != nil {
		t.Fatalf(
			"create trip location test trip: %v",
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
				"cleanup trip location test trip: %v",
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
				"cleanup trip location test ride request: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewTripLocationRepository(db)

	altitude := 42.50
	speed := 31.25
	heading := int16(180)
	accuracy := 4.75

	location1 := &models.TripLocation{
		ID:             uuid.NewString(),
		TripID:         tripID,
		DriverID:       driverID,
		Latitude:       60.169856,
		Longitude:      24.938379,
		Altitude:       &altitude,
		SpeedKMH:       &speed,
		Heading:        &heading,
		AccuracyMeters: &accuracy,
		RecordedAt:     now.Add(10 * time.Second),
	}

	location2 := &models.TripLocation{
		ID:         uuid.NewString(),
		TripID:     tripID,
		DriverID:   driverID,
		Latitude:   60.170500,
		Longitude:  24.940000,
		RecordedAt: now,
	}

	// Deliberately insert the later sample first.
	if err := repo.Create(ctx, location1); err != nil {
		t.Fatalf(
			"create first trip location: %v",
			err,
		)
	}

	if err := repo.Create(ctx, location2); err != nil {
		t.Fatalf(
			"create second trip location: %v",
			err,
		)
	}

	locations, err := repo.ListByTripID(ctx, tripID)
	if err != nil {
		t.Fatalf(
			"list trip locations: %v",
			err,
		)
	}

	if len(locations) != 2 {
		t.Fatalf(
			"expected 2 trip locations, got %d",
			len(locations),
		)
	}

	// Repository ordering must be chronological, not insertion order.
	if locations[0].ID != location2.ID {
		t.Fatalf(
			"expected first location %s, got %s",
			location2.ID,
			locations[0].ID,
		)
	}

	if locations[1].ID != location1.ID {
		t.Fatalf(
			"expected second location %s, got %s",
			location1.ID,
			locations[1].ID,
		)
	}

	got := locations[1]

	if got.Latitude != location1.Latitude {
		t.Fatalf(
			"expected latitude %f, got %f",
			location1.Latitude,
			got.Latitude,
		)
	}

	if got.Longitude != location1.Longitude {
		t.Fatalf(
			"expected longitude %f, got %f",
			location1.Longitude,
			got.Longitude,
		)
	}

	if got.Altitude == nil || *got.Altitude != altitude {
		t.Fatalf(
			"expected altitude %f, got %v",
			altitude,
			got.Altitude,
		)
	}

	if got.SpeedKMH == nil || *got.SpeedKMH != speed {
		t.Fatalf(
			"expected speed %f, got %v",
			speed,
			got.SpeedKMH,
		)
	}

	if got.Heading == nil || *got.Heading != heading {
		t.Fatalf(
			"expected heading %d, got %v",
			heading,
			got.Heading,
		)
	}

	if got.AccuracyMeters == nil || *got.AccuracyMeters != accuracy {
		t.Fatalf(
			"expected accuracy %f, got %v",
			accuracy,
			got.AccuracyMeters,
		)
	}

	// location2 intentionally omitted all nullable telemetry fields.
	first := locations[0]

	if first.Altitude != nil {
		t.Fatalf(
			"expected nil altitude, got %v",
			first.Altitude,
		)
	}

	if first.SpeedKMH != nil {
		t.Fatalf(
			"expected nil speed, got %v",
			first.SpeedKMH,
		)
	}

	if first.Heading != nil {
		t.Fatalf(
			"expected nil heading, got %v",
			first.Heading,
		)
	}

	if first.AccuracyMeters != nil {
		t.Fatalf(
			"expected nil accuracy, got %v",
			first.AccuracyMeters,
		)
	}
}
