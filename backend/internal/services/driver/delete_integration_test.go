package driver

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestDeleteDriverLifecycle(t *testing.T) {
	ctx := context.Background()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir("../../.."); err != nil {
		t.Fatalf("change to backend root: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load CONNECT configuration: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	newService := func() *Service {
		return NewService(Dependencies{
			DB:                 db,
			Drivers:            postgresrepo.NewDriverRepository(db),
			UserRoles:          repository.NewUserRoleRepository(db),
			CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
		})
	}

	createFixture := func(t *testing.T) *testutil.DriverFixture {
		t.Helper()

		fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
		if err != nil {
			t.Fatalf("create driver fixture: %v", err)
		}

		t.Cleanup(func() {
			if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
				t.Logf("cleanup driver fixture: %v", cleanupErr)
			}
		})

		return fixture
	}

	addMembership := func(t *testing.T, userID, companyID string) {
		t.Helper()

		if _, err := db.Exec(
			ctx,
			`
				INSERT INTO company_memberships (
					user_id,
					company_id
				)
				VALUES ($1, $2)
				ON CONFLICT (user_id, company_id) DO NOTHING
			`,
			userID,
			companyID,
		); err != nil {
			t.Fatalf("create company membership: %v", err)
		}

		t.Cleanup(func() {
			_, _ = db.Exec(
				context.Background(),
				`
					DELETE FROM company_memberships
					WHERE user_id=$1 AND company_id=$2
				`,
				userID,
				companyID,
			)
		})
	}

	endAssignment := func(t *testing.T, fixture *testutil.DriverFixture) {
		t.Helper()

		if _, err := db.Exec(
			ctx,
			`
				UPDATE driver_assignments
				SET unassigned_at=$2
				WHERE id=$1
			`,
			fixture.AssignmentID,
			time.Now().UTC(),
		); err != nil {
			t.Fatalf("end driver assignment: %v", err)
		}
	}

	assertNotArchived := func(
		t *testing.T,
		fixture *testutil.DriverFixture,
	) {
		t.Helper()

		var deletedAt *time.Time
		if err := db.QueryRow(
			ctx,
			`
				SELECT deleted_at
				FROM drivers
				WHERE id=$1
			`,
			fixture.DriverID,
		).Scan(&deletedAt); err != nil {
			t.Fatalf("inspect driver after rejected archive: %v", err)
		}

		if deletedAt != nil {
			t.Fatalf(
				"driver archived despite lifecycle rejection: %v",
				deletedAt,
			)
		}
	}

	t.Run("company member archives offline unassigned driver", func(t *testing.T) {
		fixture := createFixture(t)
		addMembership(t, fixture.UserID, fixture.CompanyID)
		endAssignment(t, fixture)

		if err := newService().Delete(
			ctx,
			fixture.UserID,
			fixture.DriverID,
		); err != nil {
			t.Fatalf("archive eligible driver: %v", err)
		}

		var deletedAt *time.Time
		if err := db.QueryRow(
			ctx,
			`
				SELECT deleted_at
				FROM drivers
				WHERE id=$1
			`,
			fixture.DriverID,
		).Scan(&deletedAt); err != nil {
			t.Fatalf("inspect archived driver: %v", err)
		}

		if deletedAt == nil {
			t.Fatal("expected driver deleted_at to be populated")
		}
	})

	t.Run("offline idle presence does not block archive", func(t *testing.T) {
		fixture := createFixture(t)
		addMembership(t, fixture.UserID, fixture.CompanyID)
		endAssignment(t, fixture)

		if _, err := db.Exec(
			ctx,
			`
				INSERT INTO driver_presence (
					driver_id,
					company_id,
					branch_id,
					is_online,
					availability_status
				)
				VALUES (
					$1, $2, $3, FALSE, 'OFFLINE'
				)
			`,
			fixture.UserID,
			fixture.CompanyID,
			fixture.BranchID,
		); err != nil {
			t.Fatalf("create offline driver presence: %v", err)
		}

		t.Cleanup(func() {
			_, _ = db.Exec(
				context.Background(),
				`DELETE FROM driver_presence WHERE driver_id=$1`,
				fixture.UserID,
			)
		})

		if err := newService().Delete(
			ctx,
			fixture.UserID,
			fixture.DriverID,
		); err != nil {
			t.Fatalf(
				"archive driver with offline idle presence: %v",
				err,
			)
		}

		var deletedAt *time.Time
		if err := db.QueryRow(
			ctx,
			`
				SELECT deleted_at
				FROM drivers
				WHERE id=$1
			`,
			fixture.DriverID,
		).Scan(&deletedAt); err != nil {
			t.Fatalf(
				"inspect driver archived with offline presence: %v",
				err,
			)
		}

		if deletedAt == nil {
			t.Fatal(
				"expected driver with offline idle presence to be archived",
			)
		}
	})

	t.Run("cross tenant archive is concealed as not found", func(t *testing.T) {
		target := createFixture(t)
		caller := createFixture(t)

		addMembership(t, caller.UserID, caller.CompanyID)
		endAssignment(t, target)

		err := newService().Delete(
			ctx,
			caller.UserID,
			target.DriverID,
		)
		if !errors.Is(err, ErrDriverNotFound) {
			t.Fatalf(
				"expected ErrDriverNotFound cross-tenant, got: %v",
				err,
			)
		}

		assertNotArchived(t, target)
	})

	t.Run("online presence blocks archive", func(t *testing.T) {
		fixture := createFixture(t)
		addMembership(t, fixture.UserID, fixture.CompanyID)
		endAssignment(t, fixture)

		now := time.Now().UTC()
		if _, err := db.Exec(
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
					$1, $2, $3, $4, NULL,
					TRUE, 'AVAILABLE', $5
				)
			`,
			fixture.UserID,
			fixture.CompanyID,
			fixture.BranchID,
			fixture.VehicleID,
			now,
		); err != nil {
			t.Fatalf("create online driver presence: %v", err)
		}

		t.Cleanup(func() {
			_, _ = db.Exec(
				context.Background(),
				`DELETE FROM driver_presence WHERE driver_id=$1`,
				fixture.UserID,
			)
		})

		err := newService().Delete(
			ctx,
			fixture.UserID,
			fixture.DriverID,
		)
		if !errors.Is(err, ErrDriverOperationallyActive) {
			t.Fatalf(
				"expected ErrDriverOperationallyActive, got: %v",
				err,
			)
		}

		assertNotArchived(t, fixture)
	})

	t.Run("active assignment blocks archive", func(t *testing.T) {
		fixture := createFixture(t)
		addMembership(t, fixture.UserID, fixture.CompanyID)

		err := newService().Delete(
			ctx,
			fixture.UserID,
			fixture.DriverID,
		)
		if !errors.Is(err, ErrDriverHasActiveAssignment) {
			t.Fatalf(
				"expected ErrDriverHasActiveAssignment, got: %v",
				err,
			)
		}

		assertNotArchived(t, fixture)
	})

	t.Run("active trip blocks archive", func(t *testing.T) {
		fixture := createFixture(t)
		addMembership(t, fixture.UserID, fixture.CompanyID)
		endAssignment(t, fixture)

		customerID := uuid.NewString()
		rideRequestID := uuid.NewString()
		tripID := uuid.NewString()
		now := time.Now().UTC()

		if _, err := db.Exec(
			ctx,
			`
				INSERT INTO users (
					id,
					email,
					password_hash,
					first_name,
					last_name
				)
				VALUES (
					$1,
					$2,
					'test-password-hash',
					'Archive',
					'Customer'
				)
			`,
			customerID,
			customerID+"@example.test",
		); err != nil {
			t.Fatalf("create trip customer: %v", err)
		}

		if _, err := db.Exec(
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
					'Driver Archive Pickup',
					60.2055,
					24.6559,
					'Driver Archive Destination',
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
			now.Add(10*time.Minute),
		); err != nil {
			t.Fatalf("create archive ride request: %v", err)
		}

		if _, err := db.Exec(
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
					driver_arrived_at,
					started_at,
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
					'IN_PROGRESS',
					$9,
					$9,
					$9,
					TRUE,
					$9,
					$9
				)
			`,
			tripID,
			rideRequestID,
			customerID,
			fixture.UserID,
			fixture.VehicleID,
			fixture.CompanyID,
			fixture.BranchID,
			fixture.FleetID,
			now,
		); err != nil {
			t.Fatalf("create active trip: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM trip_events WHERE trip_id=$1`,
				tripID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM trips WHERE id=$1`,
				tripID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM ride_requests WHERE id=$1`,
				rideRequestID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM users WHERE id=$1`,
				customerID,
			)
		})

		err := newService().Delete(
			ctx,
			fixture.UserID,
			fixture.DriverID,
		)
		if !errors.Is(err, ErrDriverHasActiveTrip) {
			t.Fatalf(
				"expected ErrDriverHasActiveTrip, got: %v",
				err,
			)
		}

		assertNotArchived(t, fixture)
	})

	t.Run("already archived driver is not found", func(t *testing.T) {
		fixture := createFixture(t)
		addMembership(t, fixture.UserID, fixture.CompanyID)
		endAssignment(t, fixture)

		service := newService()

		if err := service.Delete(
			ctx,
			fixture.UserID,
			fixture.DriverID,
		); err != nil {
			t.Fatalf("first archive: %v", err)
		}

		err := service.Delete(
			ctx,
			fixture.UserID,
			fixture.DriverID,
		)
		if !errors.Is(err, ErrDriverNotFound) {
			t.Fatalf(
				"expected ErrDriverNotFound on repeated archive, got: %v",
				err,
			)
		}
	})

	t.Run("nonexistent driver is not found", func(t *testing.T) {
		fixture := createFixture(t)

		err := newService().Delete(
			ctx,
			fixture.UserID,
			uuid.NewString(),
		)
		if !errors.Is(err, ErrDriverNotFound) {
			t.Fatalf(
				"expected ErrDriverNotFound for missing driver, got: %v",
				err,
			)
		}
	})
}
