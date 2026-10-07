package driver

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

type createAuthorityUserRoleRepositoryStub struct {
	roles []string
}

func (r *createAuthorityUserRoleRepositoryStub) AssignRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *createAuthorityUserRoleRepositoryStub) RemoveRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *createAuthorityUserRoleRepositoryStub) UserHasRole(
	context.Context,
	string,
	string,
) (bool, error) {
	return false, nil
}

func (r *createAuthorityUserRoleRepositoryStub) GetUserRoles(
	context.Context,
	string,
) ([]string, error) {
	return r.roles, nil
}

func TestCreateDriverLifecycleAndAuthority(t *testing.T) {
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

	createUser := func(t *testing.T) string {
		t.Helper()

		key := uuid.NewString()
		var userID string

		if err := db.QueryRow(
			ctx,
			`
				INSERT INTO users (
					email,
					password_hash,
					first_name,
					last_name,
					phone
				)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING id
			`,
			"driver-create-"+key+"@example.test",
			"test-fixture-password-hash",
			"Create",
			"Target",
			"+358"+key[:8],
		).Scan(&userID); err != nil {
			t.Fatalf("create target user: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()

			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM drivers WHERE user_id=$1`,
				userID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM users WHERE id=$1`,
				userID,
			)
		})

		return userID
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

	newService := func(roles []string) *Service {
		return NewService(Dependencies{
			DB:      db,
			Drivers: postgresrepo.NewDriverRepository(db),
			UserRoles: &createAuthorityUserRoleRepositoryStub{
				roles: roles,
			},
			CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
		})
	}

	newRequest := func(
		targetUserID string,
		companyID string,
		branchID string,
	) CreateDriverRequest {
		expiry := time.Now().UTC().AddDate(2, 0, 0)
		key := uuid.NewString()

		return CreateDriverRequest{
			UserID:       targetUserID,
			CompanyID:    companyID,
			BranchID:     branchID,
			DriverNumber: "ADM-" + key,

			FirstName: "Administrative",
			LastName:  "Driver",
			Phone:     "+358400000000",
			Email:     "administrative-" + key + "@example.test",

			TaxiDriverLicenseNumber: "TAXI-" + key,
			DrivingLicenseNumber:    "LIC-" + key,
			DrivingLicenseExpiry:    &expiry,
		}
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
			t.Fatalf("count drivers for target user: %v", err)
		}

		if count != 0 {
			t.Fatalf(
				"expected no active driver for user %s, got %d",
				userID,
				count,
			)
		}
	}

	t.Run("company member creates pending unverified driver", func(t *testing.T) {
		tenant := createFixture(t)
		caller := createFixture(t)
		targetUserID := createUser(t)

		addMembership(t, caller.UserID, tenant.CompanyID)

		created, err := newService([]string{"COMPANY_ADMIN"}).Create(
			ctx,
			caller.UserID,
			newRequest(
				targetUserID,
				tenant.CompanyID,
				tenant.BranchID,
			),
		)
		if err != nil {
			t.Fatalf("create same-tenant driver: %v", err)
		}

		if created.UserID != targetUserID {
			t.Fatalf(
				"unexpected target user: got %s want %s",
				created.UserID,
				targetUserID,
			)
		}
		if created.CompanyID != tenant.CompanyID {
			t.Fatalf(
				"unexpected company: got %s want %s",
				created.CompanyID,
				tenant.CompanyID,
			)
		}
		if created.BranchID != tenant.BranchID {
			t.Fatalf(
				"unexpected branch: got %s want %s",
				created.BranchID,
				tenant.BranchID,
			)
		}
		if created.Status != "PENDING_VERIFICATION" {
			t.Fatalf(
				"expected PENDING_VERIFICATION, got %q",
				created.Status,
			)
		}
		if created.IsVerified {
			t.Fatal("administrative create must not verify driver")
		}
		if !created.IsActive {
			t.Fatal("new driver record must be active")
		}
		if created.VerifiedAt != nil {
			t.Fatalf(
				"administrative create populated verified_at: %v",
				created.VerifiedAt,
			)
		}
		if created.VerifiedByUserID != nil {
			t.Fatalf(
				"administrative create populated verifier: %v",
				created.VerifiedByUserID,
			)
		}

		var (
			status           string
			isVerified       bool
			isActive         bool
			verifiedAt       *time.Time
			verifiedByUserID *string
		)

		if err := db.QueryRow(
			ctx,
			`
				SELECT
					status,
					is_verified,
					is_active,
					verified_at,
					verified_by_user_id
				FROM drivers
				WHERE id=$1
			`,
			created.ID,
		).Scan(
			&status,
			&isVerified,
			&isActive,
			&verifiedAt,
			&verifiedByUserID,
		); err != nil {
			t.Fatalf("inspect created driver lifecycle state: %v", err)
		}

		if status != "PENDING_VERIFICATION" ||
			isVerified ||
			!isActive ||
			verifiedAt != nil ||
			verifiedByUserID != nil {
			t.Fatalf(
				"unexpected persisted lifecycle state: "+
					"status=%q verified=%v active=%v "+
					"verified_at=%v verified_by=%v",
				status,
				isVerified,
				isActive,
				verifiedAt,
				verifiedByUserID,
			)
		}
	})

	t.Run("system admin creates globally without membership", func(t *testing.T) {
		tenant := createFixture(t)
		caller := createFixture(t)
		targetUserID := createUser(t)

		created, err := newService([]string{"SYSTEM_ADMIN"}).Create(
			ctx,
			caller.UserID,
			newRequest(
				targetUserID,
				tenant.CompanyID,
				tenant.BranchID,
			),
		)
		if err != nil {
			t.Fatalf("system admin create: %v", err)
		}

		if created.CompanyID != tenant.CompanyID ||
			created.BranchID != tenant.BranchID ||
			created.UserID != targetUserID {
			t.Fatalf("unexpected created driver: %#v", created)
		}
	})

	t.Run("cross tenant creation is denied", func(t *testing.T) {
		tenant := createFixture(t)
		caller := createFixture(t)
		targetUserID := createUser(t)

		addMembership(t, caller.UserID, caller.CompanyID)

		_, err := newService([]string{"COMPANY_ADMIN"}).Create(
			ctx,
			caller.UserID,
			newRequest(
				targetUserID,
				tenant.CompanyID,
				tenant.BranchID,
			),
		)
		if !errors.Is(err, ErrDriverCreationAccessDenied) {
			t.Fatalf(
				"expected ErrDriverCreationAccessDenied, got %v",
				err,
			)
		}

		assertNoDriverForUser(t, targetUserID)
	})

	t.Run("branch from another company is rejected", func(t *testing.T) {
		tenant := createFixture(t)
		otherTenant := createFixture(t)
		caller := createFixture(t)
		targetUserID := createUser(t)

		addMembership(t, caller.UserID, tenant.CompanyID)

		_, err := newService([]string{"COMPANY_ADMIN"}).Create(
			ctx,
			caller.UserID,
			newRequest(
				targetUserID,
				tenant.CompanyID,
				otherTenant.BranchID,
			),
		)
		if !errors.Is(err, ErrInvalidDriver) {
			t.Fatalf("expected ErrInvalidDriver, got %v", err)
		}

		assertNoDriverForUser(t, targetUserID)
	})

	t.Run("inactive branch is rejected", func(t *testing.T) {
		tenant := createFixture(t)
		caller := createFixture(t)
		targetUserID := createUser(t)

		addMembership(t, caller.UserID, tenant.CompanyID)

		if _, err := db.Exec(
			ctx,
			`UPDATE branches SET is_active=FALSE WHERE id=$1`,
			tenant.BranchID,
		); err != nil {
			t.Fatalf("deactivate branch: %v", err)
		}

		_, err := newService([]string{"COMPANY_ADMIN"}).Create(
			ctx,
			caller.UserID,
			newRequest(
				targetUserID,
				tenant.CompanyID,
				tenant.BranchID,
			),
		)
		if !errors.Is(err, ErrInvalidDriver) {
			t.Fatalf("expected ErrInvalidDriver, got %v", err)
		}

		assertNoDriverForUser(t, targetUserID)
	})

	t.Run("inactive company is rejected", func(t *testing.T) {
		tenant := createFixture(t)
		caller := createFixture(t)
		targetUserID := createUser(t)

		addMembership(t, caller.UserID, tenant.CompanyID)

		if _, err := db.Exec(
			ctx,
			`UPDATE companies SET is_active=FALSE WHERE id=$1`,
			tenant.CompanyID,
		); err != nil {
			t.Fatalf("deactivate company: %v", err)
		}

		_, err := newService([]string{"COMPANY_ADMIN"}).Create(
			ctx,
			caller.UserID,
			newRequest(
				targetUserID,
				tenant.CompanyID,
				tenant.BranchID,
			),
		)
		if !errors.Is(err, ErrInvalidDriver) {
			t.Fatalf("expected ErrInvalidDriver, got %v", err)
		}

		assertNoDriverForUser(t, targetUserID)
	})

	t.Run("duplicate active driver user is rejected deterministically", func(t *testing.T) {
		tenant := createFixture(t)
		caller := createFixture(t)

		addMembership(t, caller.UserID, tenant.CompanyID)

		_, err := newService([]string{"COMPANY_ADMIN"}).Create(
			ctx,
			caller.UserID,
			newRequest(
				tenant.UserID,
				tenant.CompanyID,
				tenant.BranchID,
			),
		)
		if !errors.Is(err, ErrDriverAlreadyExists) {
			t.Fatalf(
				"expected ErrDriverAlreadyExists, got %v",
				err,
			)
		}
	})

	t.Run("expired driving licence is rejected before persistence", func(t *testing.T) {
		tenant := createFixture(t)
		caller := createFixture(t)
		targetUserID := createUser(t)

		addMembership(t, caller.UserID, tenant.CompanyID)

		req := newRequest(
			targetUserID,
			tenant.CompanyID,
			tenant.BranchID,
		)
		expired := time.Now().UTC().Add(-24 * time.Hour)
		req.DrivingLicenseExpiry = &expired

		_, err := newService([]string{"COMPANY_ADMIN"}).Create(
			ctx,
			caller.UserID,
			req,
		)
		if !errors.Is(err, ErrInvalidDriver) {
			t.Fatalf("expected ErrInvalidDriver, got %v", err)
		}

		assertNoDriverForUser(t, targetUserID)
	})
}

func TestCreateDriverRechecksCompanyAfterLifecycleLock(t *testing.T) {
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

	targetUserID := uuid.NewString()
	targetEmail := "driver-create-race-" + targetUserID + "@example.test"

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
			VALUES (
				$1,
				$2,
				'test-fixture-password-hash',
				'Create',
				'Race',
				'+358400000001'
			)
		`,
		targetUserID,
		targetEmail,
	); err != nil {
		t.Fatalf("create race target user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM drivers WHERE user_id=$1`,
			targetUserID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id=$1`,
			targetUserID,
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

	service := NewService(Dependencies{
		DB:      db,
		Drivers: postgresrepo.NewDriverRepository(db),
		UserRoles: &createAuthorityUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
		CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
	})

	expiry := time.Now().UTC().AddDate(2, 0, 0)
	key := uuid.NewString()

	createCtx, cancelCreate := context.WithTimeout(
		ctx,
		5*time.Second,
	)
	defer cancelCreate()

	createResult := make(chan error, 1)
	go func() {
		_, createErr := service.Create(
			createCtx,
			fixture.UserID,
			CreateDriverRequest{
				UserID:       targetUserID,
				CompanyID:    fixture.CompanyID,
				BranchID:     fixture.BranchID,
				DriverNumber: "RACE-" + key,

				FirstName: "Create",
				LastName:  "Race",
				Phone:     "+358400000001",
				Email:     targetEmail,

				TaxiDriverLicenseNumber: "TAXI-RACE-" + key,
				DrivingLicenseNumber:    "LIC-RACE-" + key,
				DrivingLicenseExpiry:    &expiry,
			},
		)
		createResult <- createErr
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
		case createErr := <-createResult:
			t.Fatalf(
				"driver creation returned before controlling company lock released: %v",
				createErr,
			)
		case <-time.After(10 * time.Millisecond):
		}
	}

	if !waiting {
		t.Fatal("driver creation did not wait on company advisory lock")
	}

	companies := postgresrepo.NewCompanyRepositoryWithDB(blockerTx)
	if err := companies.Deactivate(ctx, fixture.CompanyID); err != nil {
		t.Fatalf(
			"deactivate company in controlling transaction: %v",
			err,
		)
	}

	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatalf(
			"commit controlling company deactivation: %v",
			err,
		)
	}
	blockerFinished = true

	select {
	case createErr := <-createResult:
		if !errors.Is(createErr, ErrInvalidDriver) {
			t.Fatalf(
				"expected ErrInvalidDriver after serialized company deactivation, got %v",
				createErr,
			)
		}
	case <-createCtx.Done():
		t.Fatalf(
			"driver creation did not finish after company lock release: %v",
			createCtx.Err(),
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
		targetUserID,
	).Scan(&count); err != nil {
		t.Fatalf(
			"count drivers after rejected creation: %v",
			err,
		)
	}

	if count != 0 {
		t.Fatalf(
			"expected no driver after serialized company deactivation, found %d",
			count,
		)
	}

	var companyActive bool
	if err := db.QueryRow(
		ctx,
		`SELECT is_active FROM companies WHERE id=$1`,
		fixture.CompanyID,
	).Scan(&companyActive); err != nil {
		t.Fatalf(
			"inspect company after serialized deactivation: %v",
			err,
		)
	}

	if companyActive {
		t.Fatal("company remained active after controlling deactivation")
	}
}
