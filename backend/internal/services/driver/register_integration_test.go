package driver

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
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestRegisterDriverLifecycleValidation(t *testing.T) {
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

	createUser := func(t *testing.T) *models.User {
		t.Helper()

		key := uuid.NewString()
		user := &models.User{
			BaseModel: models.BaseModel{
				ID: uuid.NewString(),
			},
			Email:     "driver-register-" + key + "@example.test",
			FirstName: "Self",
			LastName:  "Register",
			Phone:     "+358" + key[:8],
		}

		if _, err := db.Exec(
			ctx,
			`
				INSERT INTO users (
					id,
					email,
					password_hash,
					first_name,
					last_name,
					phone
				)
				VALUES ($1, $2, $3, $4, $5, $6)
			`,
			user.ID,
			user.Email,
			"test-fixture-password-hash",
			user.FirstName,
			user.LastName,
			user.Phone,
		); err != nil {
			t.Fatalf("create registration user: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()

			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM drivers WHERE user_id=$1`,
				user.ID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM users WHERE id=$1`,
				user.ID,
			)
		})

		return user
	}

	newRequest := func(
		companyID string,
		branchID string,
	) RegisterDriverRequest {
		expiry := time.Now().UTC().AddDate(2, 0, 0)
		key := uuid.NewString()

		return RegisterDriverRequest{
			CompanyID:               companyID,
			BranchID:                branchID,
			TaxiDriverLicenseNumber: "TAXI-REGISTER-" + key,
			DrivingLicenseNumber:    "LIC-REGISTER-" + key,
			DrivingLicenseExpiry:    &expiry,
		}
	}

	newService := func() *Service {
		return NewService(Dependencies{
			DB: db,
		})
	}

	assertNoDriverForUser := func(t *testing.T, userID string) {
		t.Helper()

		var count int
		if err := db.QueryRow(
			ctx,
			`
				SELECT COUNT(*)
				FROM drivers
				WHERE user_id=$1
				  AND deleted_at IS NULL
			`,
			userID,
		).Scan(&count); err != nil {
			t.Fatalf("count drivers for registration user: %v", err)
		}

		if count != 0 {
			t.Fatalf(
				"expected no active driver for user %s, got %d",
				userID,
				count,
			)
		}
	}

	t.Run("creates server-derived pending unverified driver", func(t *testing.T) {
		fixture := createFixture(t)
		user := createUser(t)

		driver, err := newService().Register(
			ctx,
			user,
			newRequest(fixture.CompanyID, fixture.BranchID),
		)
		if err != nil {
			t.Fatalf("register driver: %v", err)
		}

		if driver == nil {
			t.Fatal("expected registered driver")
		}
		if driver.UserID != user.ID {
			t.Fatalf("expected user ID %q, got %q", user.ID, driver.UserID)
		}
		if driver.CompanyID != fixture.CompanyID {
			t.Fatalf(
				"expected company ID %q, got %q",
				fixture.CompanyID,
				driver.CompanyID,
			)
		}
		if driver.BranchID != fixture.BranchID {
			t.Fatalf(
				"expected branch ID %q, got %q",
				fixture.BranchID,
				driver.BranchID,
			)
		}
		if driver.FirstName != user.FirstName ||
			driver.LastName != user.LastName ||
			driver.Email != user.Email ||
			driver.Phone != user.Phone {
			t.Fatal("expected driver identity to be derived from authenticated user")
		}
		if driver.DriverNumber == "" {
			t.Fatal("expected server-generated driver number")
		}
		if driver.Status != "PENDING_VERIFICATION" {
			t.Fatalf(
				"expected PENDING_VERIFICATION, got %q",
				driver.Status,
			)
		}
		if driver.IsVerified {
			t.Fatal("self-registered driver must not be verified")
		}
		if !driver.IsActive {
			t.Fatal("self-registered driver must be active")
		}

		persisted, err := postgresrepo.NewDriverRepository(db).GetByUserID(
			ctx,
			user.ID,
		)
		if err != nil {
			t.Fatalf("get persisted driver: %v", err)
		}
		if persisted.ID != driver.ID {
			t.Fatalf(
				"expected persisted driver %q, got %q",
				driver.ID,
				persisted.ID,
			)
		}
	})

	t.Run("rejects inactive company", func(t *testing.T) {
		fixture := createFixture(t)
		user := createUser(t)

		// Company deactivation normally requires no active branches. Directly
		// set lifecycle state here to exercise Register's authoritative company
		// validation independently from the company service workflow.
		if _, err := db.Exec(
			ctx,
			`UPDATE companies SET is_active=FALSE WHERE id=$1`,
			fixture.CompanyID,
		); err != nil {
			t.Fatalf("make registration company inactive: %v", err)
		}

		_, err := newService().Register(
			ctx,
			user,
			newRequest(fixture.CompanyID, fixture.BranchID),
		)
		if !errors.Is(err, ErrInvalidDriver) {
			t.Fatalf("expected ErrInvalidDriver, got %v", err)
		}

		assertNoDriverForUser(t, user.ID)
	})

	t.Run("rejects inactive branch", func(t *testing.T) {
		fixture := createFixture(t)
		user := createUser(t)

		if _, err := db.Exec(
			ctx,
			`UPDATE branches SET is_active=FALSE WHERE id=$1`,
			fixture.BranchID,
		); err != nil {
			t.Fatalf("make registration branch inactive: %v", err)
		}

		_, err := newService().Register(
			ctx,
			user,
			newRequest(fixture.CompanyID, fixture.BranchID),
		)
		if !errors.Is(err, ErrInvalidDriver) {
			t.Fatalf("expected ErrInvalidDriver, got %v", err)
		}

		assertNoDriverForUser(t, user.ID)
	})

	t.Run("rejects branch owned by different company", func(t *testing.T) {
		tenant := createFixture(t)
		otherTenant := createFixture(t)
		user := createUser(t)

		_, err := newService().Register(
			ctx,
			user,
			newRequest(tenant.CompanyID, otherTenant.BranchID),
		)
		if !errors.Is(err, ErrInvalidDriver) {
			t.Fatalf("expected ErrInvalidDriver, got %v", err)
		}

		assertNoDriverForUser(t, user.ID)
	})

	t.Run("rejects existing active driver", func(t *testing.T) {
		fixture := createFixture(t)

		user := &models.User{
			BaseModel: models.BaseModel{ID: fixture.UserID},
		}

		_, err := newService().Register(
			ctx,
			user,
			newRequest(fixture.CompanyID, fixture.BranchID),
		)
		if !errors.Is(err, ErrDriverAlreadyExists) {
			t.Fatalf(
				"expected ErrDriverAlreadyExists, got %v",
				err,
			)
		}
	})
}

func TestRegisterDriverRechecksBranchAfterLifecycleLock(t *testing.T) {
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

	fixture, cleanup, err := testutil.CreateDriverFixture(ctx, db)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}
	t.Cleanup(func() {
		if cleanupErr := cleanup(context.Background()); cleanupErr != nil {
			t.Logf("cleanup driver fixture: %v", cleanupErr)
		}
	})

	userID := uuid.NewString()
	key := uuid.NewString()
	user := &models.User{
		BaseModel: models.BaseModel{ID: userID},
		Email:     "driver-register-race-" + key + "@example.test",
		FirstName: "Register",
		LastName:  "Race",
		Phone:     "+358" + key[:8],
	}

	if _, err := db.Exec(
		ctx,
		`
			INSERT INTO users (
				id,
				email,
				password_hash,
				first_name,
				last_name,
				phone
			)
			VALUES ($1, $2, $3, $4, $5, $6)
		`,
		user.ID,
		user.Email,
		"test-fixture-password-hash",
		user.FirstName,
		user.LastName,
		user.Phone,
	); err != nil {
		t.Fatalf("create race registration user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM drivers WHERE user_id=$1`,
			user.ID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id=$1`,
			user.ID,
		)
	})

	blockerTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin controlling transaction: %v", err)
	}

	blockerFinished := false
	t.Cleanup(func() {
		if !blockerFinished {
			_ = blockerTx.Rollback(context.Background())
		}
	})

	if err := postgresrepo.AcquireTransactionAdvisoryLock(
		ctx,
		blockerTx,
		"company:"+fixture.CompanyID,
	); err != nil {
		t.Fatalf("acquire controlling company lock: %v", err)
	}

	expiry := time.Now().UTC().AddDate(2, 0, 0)
	registerCtx, cancelRegister := context.WithTimeout(ctx, 5*time.Second)
	defer cancelRegister()

	registerResult := make(chan error, 1)
	go func() {
		_, registerErr := NewService(Dependencies{
			DB: db,
		}).Register(
			registerCtx,
			user,
			RegisterDriverRequest{
				CompanyID:               fixture.CompanyID,
				BranchID:                fixture.BranchID,
				TaxiDriverLicenseNumber: "TAXI-RACE-" + key,
				DrivingLicenseNumber:    "LIC-RACE-" + key,
				DrivingLicenseExpiry:    &expiry,
			},
		)
		registerResult <- registerErr
	}()

	var waiting bool
	for attempt := 0; attempt < 100; attempt++ {
		if err := db.QueryRow(
			ctx,
			`
				SELECT EXISTS (
					SELECT 1
					FROM pg_locks
					WHERE locktype='advisory'
					  AND NOT granted
				)
			`,
		).Scan(&waiting); err != nil {
			t.Fatalf("inspect advisory lock waiters: %v", err)
		}

		if waiting {
			break
		}

		select {
		case registerErr := <-registerResult:
			t.Fatalf(
				"driver registration returned before company lock released: %v",
				registerErr,
			)
		case <-time.After(10 * time.Millisecond):
		}
	}

	if !waiting {
		t.Fatal("driver registration did not wait on company lifecycle lock")
	}

	branches := postgresrepo.NewBranchRepositoryWithDB(blockerTx)
	if err := branches.Deactivate(ctx, fixture.BranchID); err != nil {
		t.Fatalf(
			"deactivate branch in controlling transaction: %v",
			err,
		)
	}

	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatalf("commit controlling branch deactivation: %v", err)
	}
	blockerFinished = true

	select {
	case registerErr := <-registerResult:
		if !errors.Is(registerErr, ErrInvalidDriver) {
			t.Fatalf(
				"expected ErrInvalidDriver after serialized branch deactivation, got %v",
				registerErr,
			)
		}
	case <-registerCtx.Done():
		t.Fatalf(
			"driver registration did not finish after company lock release: %v",
			registerCtx.Err(),
		)
	}

	var count int
	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM drivers
			WHERE user_id=$1
			  AND deleted_at IS NULL
		`,
		user.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count drivers after rejected registration: %v", err)
	}

	if count != 0 {
		t.Fatalf(
			"expected no driver after serialized branch deactivation, found %d",
			count,
		)
	}
}
