package trip

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
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestUpdateAuthorizedEnforcesTenantMembership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if os.Getenv("DB_NAME") != "connect_integration_test" {
		t.Fatal("refusing tenant update test outside integration database")
	}

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
		t.Fatalf("load configuration: %v", err)
	}
	if cfg.Database.Name != "connect_integration_test" {
		t.Fatalf("unsafe database: %s", cfg.Database.Name)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	defer db.Close()

	releaseFixtureLock, err := testutil.AcquirePostgresFixtureLock(
		ctx, db, "dispatch-fixture:john",
	)
	if err != nil {
		t.Fatalf("acquire fixture lock: %v", err)
	}
	defer func() {
		if err := releaseFixtureLock(context.Background()); err != nil {
			t.Errorf("release fixture lock: %v", err)
		}
	}()

	customerID, cleanupCustomer, err := testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create customer fixture: %v", err)
	}
	defer func() {
		if err := cleanupCustomer(context.Background()); err != nil {
			t.Errorf("cleanup customer fixture: %v", err)
		}
	}()

	driver, cleanupDriver, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	defer func() {
		if err := cleanupDriver(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	}()

	adminID, cleanupAdmin, err := testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create administrator fixture: %v", err)
	}
	defer func() {
		if err := cleanupAdmin(context.Background()); err != nil {
			t.Errorf("cleanup administrator fixture: %v", err)
		}
	}()

	outsiderID, cleanupOutsider, err := testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatalf("create outsider fixture: %v", err)
	}
	defer func() {
		if err := cleanupOutsider(context.Background()); err != nil {
			t.Errorf("cleanup outsider fixture: %v", err)
		}
	}()

	var roleID string
	if err := db.QueryRow(ctx, `
        SELECT id FROM roles WHERE name = 'COMPANY_ADMIN'
    `).Scan(&roleID); err != nil {
		t.Fatalf("resolve COMPANY_ADMIN role: %v", err)
	}

	for _, userID := range []string{adminID, outsiderID} {
		if _, err := db.Exec(ctx, `
            INSERT INTO user_roles (user_id, role_id)
            VALUES ($1, $2)
        `, userID, roleID); err != nil {
			t.Fatalf("assign company administrator role: %v", err)
		}
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		for _, userID := range []string{adminID, outsiderID} {
			if _, err := db.Exec(cleanupCtx, `
                DELETE FROM user_roles
                WHERE user_id = $1 AND role_id = $2
            `, userID, roleID); err != nil {
				t.Errorf("cleanup administrator role: %v", err)
			}
		}
	}()

	if _, err := db.Exec(ctx, `
        INSERT INTO company_memberships (user_id, company_id)
        VALUES ($1, $2)
    `, adminID, driver.CompanyID); err != nil {
		t.Fatalf("create administrator membership: %v", err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		if _, err := db.Exec(cleanupCtx, `
            DELETE FROM company_memberships
            WHERE user_id = $1 AND company_id = $2
        `, adminID, driver.CompanyID); err != nil {
			t.Errorf("cleanup membership: %v", err)
		}
	}()

	rideRequestID := uuid.NewString()
	tripID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if _, err := db.Exec(ctx, `
        INSERT INTO ride_requests (
            id, customer_id,
            pickup_address, pickup_latitude, pickup_longitude,
            destination_address, destination_latitude,
            destination_longitude, requested_vehicle_type,
            passenger_count, status, requested_at,
            expires_at, created_at, updated_at
        )
        VALUES (
            $1, $2,
            'Tenant Update Pickup', 60.169856, 24.938379,
            'Tenant Update Destination', 60.170500,
            24.940000, 'STANDARD',
            1, 'ACCEPTED', $3, $4, $3, $3
        )
    `, rideRequestID, customerID, now, now.Add(30*time.Minute)); err != nil {
		t.Fatalf("create ride request: %v", err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		if _, err := db.Exec(cleanupCtx,
			`DELETE FROM trips WHERE id = $1`, tripID,
		); err != nil {
			t.Errorf("cleanup trip: %v", err)
		}
		if _, err := db.Exec(cleanupCtx,
			`DELETE FROM ride_requests WHERE id = $1`, rideRequestID,
		); err != nil {
			t.Errorf("cleanup ride request: %v", err)
		}
	}()

	initialNote := "initial tenant note"

	if _, err := db.Exec(ctx, `
        INSERT INTO trips (
            id, ride_request_id, customer_id,
            driver_id, vehicle_id, company_id,
            branch_id, fleet_id, status,
            assigned_at, passenger_note,
            is_active, created_at, updated_at
        )
        VALUES (
            $1, $2, $3,
            $4, $5, $6,
            $7, $8, 'ASSIGNED',
            $9, $10,
            TRUE, $9, $9
        )
    `,
		tripID, rideRequestID, customerID,
		driver.UserID, driver.VehicleID, driver.CompanyID,
		driver.BranchID, driver.FleetID,
		now, initialNote,
	); err != nil {
		t.Fatalf("create trip: %v", err)
	}

	service := NewService(Dependencies{
		DB:        db,
		Trips:     postgresrepo.NewTripRepository(db),
		UserRoles: repository.NewUserRoleRepository(db),
	})

	readNote := func() string {
		t.Helper()

		var note string
		if err := db.QueryRow(ctx, `
            SELECT passenger_note
            FROM trips
            WHERE id = $1
        `, tripID).Scan(&note); err != nil {
			t.Fatalf("read persisted passenger note: %v", err)
		}
		return note
	}

	authorizedNote := "authorized company update"

	if err := service.UpdateAuthorized(
		ctx,
		&models.Trip{
			BaseModel:     models.BaseModel{ID: tripID},
			PassengerNote: &authorizedNote,
		},
		adminID,
	); err != nil {
		t.Fatalf("authorized company update failed: %v", err)
	}

	if got := readNote(); got != authorizedNote {
		t.Fatalf("authorized update not persisted: %q", got)
	}

	crossCompanyNote := "unauthorized outsider update"

	err = service.UpdateAuthorized(
		ctx,
		&models.Trip{
			BaseModel:     models.BaseModel{ID: tripID},
			PassengerNote: &crossCompanyNote,
		},
		outsiderID,
	)
	if !errors.Is(err, ErrTripMutationAccessDenied) {
		t.Fatalf("expected cross-company access denial, got %v", err)
	}
	if got := readNote(); got != authorizedNote {
		t.Fatalf("cross-company denial changed persisted note: %q", got)
	}

	if _, err := db.Exec(ctx, `
        DELETE FROM company_memberships
        WHERE user_id = $1 AND company_id = $2
    `, adminID, driver.CompanyID); err != nil {
		t.Fatalf("revoke administrator membership: %v", err)
	}

	// Concurrent revocation while update waits on the authoritative trip row.
	// Confirm the service is blocked before revoking membership.
	if _, err := db.Exec(ctx, `
        INSERT INTO company_memberships (user_id, company_id)
        VALUES ($1, $2)
    `, adminID, driver.CompanyID); err != nil {
		t.Fatalf("restore membership for concurrency test: %v", err)
	}

	holder, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin trip lock transaction: %v", err)
	}
	defer func() {
		_ = holder.Rollback(context.Background())
	}()

	var holderPID int
	if err := holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatalf("identify trip lock holder: %v", err)
	}

	if _, err := holder.Exec(ctx, `
        SELECT id FROM trips WHERE id = $1 FOR UPDATE
    `, tripID); err != nil {
		t.Fatalf("lock trip before concurrent update: %v", err)
	}

	concurrentNote := "concurrent update must be denied"
	updateResult := make(chan error, 1)

	go func() {
		updateResult <- service.UpdateAuthorized(
			ctx,
			&models.Trip{
				BaseModel:     models.BaseModel{ID: tripID},
				PassengerNote: &concurrentNote,
			},
			adminID,
		)
	}()

	observedBlocked := false
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		var waiting int
		err := db.QueryRow(ctx, `
            SELECT COUNT(*)
            FROM pg_stat_activity
            WHERE pid <> $1
              AND datname = current_database()
              AND state = 'active'
              AND wait_event_type = 'Lock'
              AND $1 = ANY(pg_blocking_pids(pid))
              AND query LIKE '%FROM trips%'
        `, holderPID).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect blocked trip update: %v", err)
		}
		if waiting > 0 {
			observedBlocked = true
			break
		}

		select {
		case err := <-updateResult:
			t.Fatalf("update finished before trip lock release: %v", err)
		case <-time.After(25 * time.Millisecond):
		}
	}

	if !observedBlocked {
		t.Fatal("did not observe update waiting on authoritative trip lock")
	}

	tag, err := db.Exec(ctx, `
        DELETE FROM company_memberships
        WHERE user_id = $1 AND company_id = $2
    `, adminID, driver.CompanyID)
	if err != nil {
		t.Fatalf("revoke membership while update waits: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected one revoked membership, got %d", tag.RowsAffected())
	}

	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("release trip lock: %v", err)
	}

	select {
	case err := <-updateResult:
		if !errors.Is(err, ErrTripMutationAccessDenied) {
			t.Fatalf("expected concurrent revocation denial, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent update did not finish after trip lock release")
	}

	if got := readNote(); got != authorizedNote {
		t.Fatalf("concurrent revoked update changed persisted note: %q", got)
	}

	revokedNote := "revoked administrator update"

	err = service.UpdateAuthorized(
		ctx,
		&models.Trip{
			BaseModel:     models.BaseModel{ID: tripID},
			PassengerNote: &revokedNote,
		},
		adminID,
	)
	if !errors.Is(err, ErrTripMutationAccessDenied) {
		t.Fatalf("expected revoked membership denial, got %v", err)
	}
	if got := readNote(); got != authorizedNote {
		t.Fatalf("revoked administrator changed persisted note: %q", got)
	}
}
