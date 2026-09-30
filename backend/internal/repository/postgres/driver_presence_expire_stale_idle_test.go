package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestDriverPresenceRepositoryExpireStaleIdle(t *testing.T) {
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

	repo := NewDriverPresenceRepository(db)

	type testCase struct {
		name             string
		status           string
		isOnline         bool
		heartbeat        *time.Time
		wantTransitioned bool
		wantOnline       bool
		wantStatus       string
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	staleBefore := now.Add(-2 * time.Minute)

	staleHeartbeat := staleBefore.Add(-time.Second)
	exactCutoffHeartbeat := staleBefore
	freshHeartbeat := staleBefore.Add(time.Second)

	tests := []testCase{
		{
			name:             "stale available driver moves offline",
			status:           "AVAILABLE",
			isOnline:         true,
			heartbeat:        &staleHeartbeat,
			wantTransitioned: true,
			wantOnline:       false,
			wantStatus:       "OFFLINE",
		},
		{
			name:             "stale break driver moves offline",
			status:           "BREAK",
			isOnline:         true,
			heartbeat:        &staleHeartbeat,
			wantTransitioned: true,
			wantOnline:       false,
			wantStatus:       "OFFLINE",
		},
		{
			name:             "null heartbeat moves idle online driver offline",
			status:           "AVAILABLE",
			isOnline:         true,
			heartbeat:        nil,
			wantTransitioned: true,
			wantOnline:       false,
			wantStatus:       "OFFLINE",
		},
		{
			name:             "exact cutoff remains online",
			status:           "AVAILABLE",
			isOnline:         true,
			heartbeat:        &exactCutoffHeartbeat,
			wantTransitioned: false,
			wantOnline:       true,
			wantStatus:       "AVAILABLE",
		},
		{
			name:             "fresh heartbeat remains online",
			status:           "AVAILABLE",
			isOnline:         true,
			heartbeat:        &freshHeartbeat,
			wantTransitioned: false,
			wantOnline:       true,
			wantStatus:       "AVAILABLE",
		},
		{
			name:             "busy driver remains online even when stale",
			status:           "BUSY",
			isOnline:         true,
			heartbeat:        &staleHeartbeat,
			wantTransitioned: false,
			wantOnline:       true,
			wantStatus:       "BUSY",
		},
		{
			name:             "already offline driver remains offline",
			status:           "OFFLINE",
			isOnline:         false,
			heartbeat:        &staleHeartbeat,
			wantTransitioned: false,
			wantOnline:       false,
			wantStatus:       "OFFLINE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture, cleanupFixture, err :=
				testutil.CreateDriverFixture(ctx, db)
			if err != nil {
				t.Fatalf("create driver fixture: %v", err)
			}
			defer func() {
				if cleanupErr := cleanupFixture(
					context.Background(),
				); cleanupErr != nil {
					t.Logf(
						"cleanup driver fixture: %v",
						cleanupErr,
					)
				}
			}()

			_, err = db.Exec(
				ctx,
				`
					INSERT INTO driver_presence (
						driver_id,
						company_id,
						branch_id,
						vehicle_id,
						assignment_id,
						is_online,
						availability_status,
						last_heartbeat_at
					)
					VALUES (
						$1,
						$2,
						$3,
						$4,
						$5,
						$6,
						$7,
						$8
					)
				`,
				fixture.UserID,
				fixture.CompanyID,
				fixture.BranchID,
				fixture.VehicleID,
				fixture.AssignmentID,
				tc.isOnline,
				tc.status,
				tc.heartbeat,
			)
			if err != nil {
				t.Fatalf("create driver presence: %v", err)
			}

			defer func() {
				if _, cleanupErr := db.Exec(
					context.Background(),
					`DELETE FROM driver_presence WHERE driver_id = $1`,
					fixture.UserID,
				); cleanupErr != nil {
					t.Logf(
						"cleanup driver presence: %v",
						cleanupErr,
					)
				}
			}()

			transitioned, err := repo.ExpireStaleIdle(
				ctx,
				fixture.UserID,
				staleBefore,
			)
			if err != nil {
				t.Fatalf("expire stale idle presence: %v", err)
			}

			if transitioned != tc.wantTransitioned {
				t.Fatalf(
					"expected transitioned=%v, got %v",
					tc.wantTransitioned,
					transitioned,
				)
			}

			var (
				persistedOnline bool
				persistedStatus string
			)

			err = db.QueryRow(
				ctx,
				`
					SELECT
						is_online,
						availability_status
					FROM driver_presence
					WHERE driver_id = $1
				`,
				fixture.UserID,
			).Scan(
				&persistedOnline,
				&persistedStatus,
			)
			if err != nil {
				t.Fatalf("read persisted driver presence: %v", err)
			}

			if persistedOnline != tc.wantOnline {
				t.Fatalf(
					"expected is_online=%v, got %v",
					tc.wantOnline,
					persistedOnline,
				)
			}

			if persistedStatus != tc.wantStatus {
				t.Fatalf(
					"expected status %q, got %q",
					tc.wantStatus,
					persistedStatus,
				)
			}
		})
	}
}

func TestDriverPresenceRepositoryExpireStaleIdlePreservesActiveTrip(t *testing.T) {
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

	fixture, cleanupFixture, err :=
		testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if cleanupErr := cleanupFixture(
			context.Background(),
		); cleanupErr != nil {
			t.Logf(
				"cleanup driver fixture: %v",
				cleanupErr,
			)
		}
	}()

	now := time.Now().UTC().Truncate(time.Microsecond)
	staleBefore := now.Add(-2 * time.Minute)
	staleHeartbeat := staleBefore.Add(-time.Second)

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO driver_presence (
				driver_id,
				company_id,
				branch_id,
				vehicle_id,
				assignment_id,
				is_online,
				availability_status,
				last_heartbeat_at
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				TRUE,
				'AVAILABLE',
				$6
			)
		`,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		staleHeartbeat,
	)
	if err != nil {
		t.Fatalf("create stale driver presence: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM driver_presence WHERE driver_id = $1`,
			fixture.UserID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup driver presence: %v",
				cleanupErr,
			)
		}
	}()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()

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
				'Stale Presence Test Pickup',
				60.2055,
				24.6559,
				'Stale Presence Test Destination',
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
		fixture.UserID,
		now,
		now.Add(10*time.Minute),
	)
	if err != nil {
		t.Fatalf("create ride request: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM ride_requests WHERE id = $1`,
			rideRequestID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup ride request: %v",
				cleanupErr,
			)
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
				assigned_at,
				is_active,
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
				'ASSIGNED',
				$9,
				TRUE,
				$9,
				$9
			)
		`,
		tripID,
		rideRequestID,
		fixture.UserID,
		fixture.UserID,
		fixture.VehicleID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.FleetID,
		now,
	)
	if err != nil {
		t.Fatalf("create active trip: %v", err)
	}

	defer func() {
		if _, cleanupErr := db.Exec(
			context.Background(),
			`DELETE FROM trips WHERE id = $1`,
			tripID,
		); cleanupErr != nil {
			t.Logf(
				"cleanup active trip: %v",
				cleanupErr,
			)
		}
	}()

	repo := NewDriverPresenceRepository(db)

	transitioned, err := repo.ExpireStaleIdle(
		ctx,
		fixture.UserID,
		staleBefore,
	)
	if err != nil {
		t.Fatalf("expire stale idle presence: %v", err)
	}

	if transitioned {
		t.Fatal(
			"expected active trip to prevent stale presence expiration",
		)
	}

	var (
		persistedOnline bool
		persistedStatus string
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				is_online,
				availability_status
			FROM driver_presence
			WHERE driver_id = $1
		`,
		fixture.UserID,
	).Scan(
		&persistedOnline,
		&persistedStatus,
	)
	if err != nil {
		t.Fatalf("read persisted driver presence: %v", err)
	}

	if !persistedOnline {
		t.Fatal(
			"expected active-trip driver to remain online",
		)
	}

	if persistedStatus != "AVAILABLE" {
		t.Fatalf(
			"expected status AVAILABLE, got %q",
			persistedStatus,
		)
	}
}

func TestDriverPresenceRepositoryExpireAllStaleIdle(t *testing.T) {
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

	now := time.Now().UTC().Truncate(time.Microsecond)
	staleBefore := now.Add(-2 * time.Minute)

	staleHeartbeat := staleBefore.Add(-time.Second)
	exactCutoffHeartbeat := staleBefore
	freshHeartbeat := staleBefore.Add(time.Second)

	var baselineEligible int64
	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM driver_presence AS dp
			WHERE dp.is_online = TRUE
			  AND dp.availability_status IN ('AVAILABLE', 'BREAK')
			  AND (
				dp.last_heartbeat_at IS NULL
				OR dp.last_heartbeat_at < $1
			  )
			  AND NOT EXISTS (
				SELECT 1
				FROM trips AS t
				WHERE t.driver_id = dp.driver_id
				  AND t.is_active = TRUE
				  AND t.deleted_at IS NULL
				  AND t.status NOT IN (
					'COMPLETED',
					'CANCELLED',
					'NO_DRIVER_AVAILABLE',
					'EXPIRED'
				  )
			  )
		`,
		staleBefore,
	).Scan(&baselineEligible)
	if err != nil {
		t.Fatalf(
			"count pre-existing stale idle presence: %v",
			err,
		)
	}

	type presenceCase struct {
		name       string
		status     string
		isOnline   bool
		heartbeat  *time.Time
		wantOnline bool
		wantStatus string
		activeTrip bool
	}

	cases := []presenceCase{
		{
			name:       "stale available",
			status:     "AVAILABLE",
			isOnline:   true,
			heartbeat:  &staleHeartbeat,
			wantOnline: false,
			wantStatus: "OFFLINE",
		},
		{
			name:       "stale break",
			status:     "BREAK",
			isOnline:   true,
			heartbeat:  &staleHeartbeat,
			wantOnline: false,
			wantStatus: "OFFLINE",
		},
		{
			name:       "null heartbeat",
			status:     "AVAILABLE",
			isOnline:   true,
			heartbeat:  nil,
			wantOnline: false,
			wantStatus: "OFFLINE",
		},
		{
			name:       "exact cutoff",
			status:     "AVAILABLE",
			isOnline:   true,
			heartbeat:  &exactCutoffHeartbeat,
			wantOnline: true,
			wantStatus: "AVAILABLE",
		},
		{
			name:       "fresh heartbeat",
			status:     "AVAILABLE",
			isOnline:   true,
			heartbeat:  &freshHeartbeat,
			wantOnline: true,
			wantStatus: "AVAILABLE",
		},
		{
			name:       "stale busy",
			status:     "BUSY",
			isOnline:   true,
			heartbeat:  &staleHeartbeat,
			wantOnline: true,
			wantStatus: "BUSY",
		},
		{
			name:       "stale available with active trip",
			status:     "AVAILABLE",
			isOnline:   true,
			heartbeat:  &staleHeartbeat,
			wantOnline: true,
			wantStatus: "AVAILABLE",
			activeTrip: true,
		},
	}

	type createdCase struct {
		tc            presenceCase
		fixture       *testutil.DriverFixture
		cleanup       func(context.Context) error
		rideRequestID string
		tripID        string
	}

	created := make([]createdCase, 0, len(cases))

	for _, tc := range cases {
		fixture, cleanupFixture, err :=
			testutil.CreateDriverFixture(ctx, db)
		if err != nil {
			t.Fatalf(
				"create driver fixture for %q: %v",
				tc.name,
				err,
			)
		}

		_, err = db.Exec(
			ctx,
			`
				INSERT INTO driver_presence (
					driver_id,
					company_id,
					branch_id,
					vehicle_id,
					assignment_id,
					is_online,
					availability_status,
					last_heartbeat_at
				)
				VALUES (
					$1,
					$2,
					$3,
					$4,
					$5,
					$6,
					$7,
					$8
				)
			`,
			fixture.UserID,
			fixture.CompanyID,
			fixture.BranchID,
			fixture.VehicleID,
			fixture.AssignmentID,
			tc.isOnline,
			tc.status,
			tc.heartbeat,
		)
		if err != nil {
			_ = cleanupFixture(context.Background())
			t.Fatalf(
				"create driver presence for %q: %v",
				tc.name,
				err,
			)
		}

		item := createdCase{
			tc:      tc,
			fixture: fixture,
			cleanup: cleanupFixture,
		}

		if tc.activeTrip {
			item.rideRequestID = uuid.NewString()
			item.tripID = uuid.NewString()

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
						'Batch Stale Presence Pickup',
						60.2055,
						24.6559,
						'Batch Stale Presence Destination',
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
				item.rideRequestID,
				fixture.UserID,
				now,
				now.Add(10*time.Minute),
			)
			if err != nil {
				t.Fatalf(
					"create ride request for %q: %v",
					tc.name,
					err,
				)
			}

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
						assigned_at,
						is_active,
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
						'ASSIGNED',
						$9,
						TRUE,
						$9,
						$9
					)
				`,
				item.tripID,
				item.rideRequestID,
				fixture.UserID,
				fixture.UserID,
				fixture.VehicleID,
				fixture.CompanyID,
				fixture.BranchID,
				fixture.FleetID,
				now,
			)
			if err != nil {
				t.Fatalf(
					"create active trip for %q: %v",
					tc.name,
					err,
				)
			}
		}

		created = append(created, item)
	}

	defer func() {
		for i := len(created) - 1; i >= 0; i-- {
			item := created[i]

			if item.tripID != "" {
				if _, cleanupErr := db.Exec(
					context.Background(),
					`DELETE FROM trips WHERE id = $1`,
					item.tripID,
				); cleanupErr != nil {
					t.Logf(
						"cleanup trip for %q: %v",
						item.tc.name,
						cleanupErr,
					)
				}
			}

			if item.rideRequestID != "" {
				if _, cleanupErr := db.Exec(
					context.Background(),
					`DELETE FROM ride_requests WHERE id = $1`,
					item.rideRequestID,
				); cleanupErr != nil {
					t.Logf(
						"cleanup ride request for %q: %v",
						item.tc.name,
						cleanupErr,
					)
				}
			}

			if _, cleanupErr := db.Exec(
				context.Background(),
				`DELETE FROM driver_presence WHERE driver_id = $1`,
				item.fixture.UserID,
			); cleanupErr != nil {
				t.Logf(
					"cleanup presence for %q: %v",
					item.tc.name,
					cleanupErr,
				)
			}

			if cleanupErr := item.cleanup(
				context.Background(),
			); cleanupErr != nil {
				t.Logf(
					"cleanup fixture for %q: %v",
					item.tc.name,
					cleanupErr,
				)
			}
		}
	}()

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin batch expiration transaction: %v", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil {
			t.Logf(
				"rollback batch expiration transaction: %v",
				rollbackErr,
			)
		}
	}()

	repo := NewDriverPresenceRepositoryWithDB(tx)

	expiredCount, err := repo.ExpireAllStaleIdle(
		ctx,
		staleBefore,
	)
	if err != nil {
		t.Fatalf("expire all stale idle presence: %v", err)
	}

	const fixtureExpiredCount int64 = 3
	wantExpiredCount := baselineEligible + fixtureExpiredCount

	if expiredCount != wantExpiredCount {
		t.Fatalf(
			"expected %d expired presence rows "+
				"(%d pre-existing + %d fixtures), got %d",
			wantExpiredCount,
			baselineEligible,
			fixtureExpiredCount,
			expiredCount,
		)
	}

	for _, item := range created {
		var (
			persistedOnline bool
			persistedStatus string
		)

		err = tx.QueryRow(
			ctx,
			`
				SELECT
					is_online,
					availability_status
				FROM driver_presence
				WHERE driver_id = $1
			`,
			item.fixture.UserID,
		).Scan(
			&persistedOnline,
			&persistedStatus,
		)
		if err != nil {
			t.Fatalf(
				"read persisted presence for %q: %v",
				item.tc.name,
				err,
			)
		}

		if persistedOnline != item.tc.wantOnline {
			t.Errorf(
				"%q: expected is_online=%v, got %v",
				item.tc.name,
				item.tc.wantOnline,
				persistedOnline,
			)
		}

		if persistedStatus != item.tc.wantStatus {
			t.Errorf(
				"%q: expected status %q, got %q",
				item.tc.name,
				item.tc.wantStatus,
				persistedStatus,
			)
		}
	}
}
