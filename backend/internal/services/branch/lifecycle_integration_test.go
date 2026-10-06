package branch

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
	"github.com/google/uuid"
)

type branchLifecycleFixture struct {
	UserID    string
	CompanyID string
	BranchID  string
}

func TestBranchLifecycleEnforcesTenantAndFleetAuthority(t *testing.T) {
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

	createFixture := func(t *testing.T) branchLifecycleFixture {
		t.Helper()

		f := branchLifecycleFixture{
			UserID:    uuid.NewString(),
			CompanyID: uuid.NewString(),
			BranchID:  uuid.NewString(),
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO users (
				id, email, password_hash, first_name, last_name
			)
			VALUES ($1, $2, 'test-password-hash', 'Branch', 'Lifecycle')
		`, f.UserID, f.UserID+"@example.test"); err != nil {
			t.Fatalf("create lifecycle user: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO companies (
				id, name, legal_name, business_id, email,
				country_code, timezone
			)
			VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
		`,
			f.CompanyID,
			"Branch Lifecycle "+f.CompanyID[:8],
			"Branch Lifecycle "+f.CompanyID[:8]+" Oy",
			"BRL-"+f.CompanyID[:8],
			f.CompanyID+"@example.test",
		); err != nil {
			t.Fatalf("create lifecycle company: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO branches (
				id, company_id, code, name, is_active
			)
			VALUES ($1, $2, $3, $4, TRUE)
		`,
			f.BranchID,
			f.CompanyID,
			"BR-"+f.BranchID[:8],
			"Branch Lifecycle "+f.BranchID[:8],
		); err != nil {
			t.Fatalf("create lifecycle branch: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO company_memberships (user_id, company_id)
			VALUES ($1, $2)
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("create lifecycle membership: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM fleets WHERE branch_id=$1`,
				f.BranchID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM company_memberships WHERE user_id=$1`,
				f.UserID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM branches WHERE id=$1`,
				f.BranchID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM companies WHERE id=$1`,
				f.CompanyID,
			)
			_, _ = db.Exec(
				cleanupCtx,
				`DELETE FROM users WHERE id=$1`,
				f.UserID,
			)
		})

		return f
	}

	createFleet := func(
		t *testing.T,
		f branchLifecycleFixture,
		active bool,
	) string {
		t.Helper()

		fleetID := uuid.NewString()
		if _, err := db.Exec(ctx, `
			INSERT INTO fleets (
				id,
				company_id,
				branch_id,
				code,
				name,
				description,
				is_active
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`,
			fleetID,
			f.CompanyID,
			f.BranchID,
			"FL-"+fleetID[:8],
			"Branch Lifecycle Fleet "+fleetID[:8],
			"branch lifecycle integration test",
			active,
		); err != nil {
			t.Fatalf("create lifecycle fleet: %v", err)
		}

		return fleetID
	}

	newService := func(roles []string) *Service {
		return NewService(Dependencies{
			DB:       db,
			Branches: postgresrepo.NewBranchRepository(db),
			UserRoles: &branchAuthorityUserRoleRepositoryStub{
				roles: roles,
			},
			CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
		})
	}

	t.Run("member deactivates empty branch", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if err := service.Deactivate(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("deactivate own empty branch: %v", err)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect branch state: %v", err)
		}
		if active {
			t.Fatal("branch remained active after deactivation")
		}
	})

	t.Run("inactive fleet does not block branch deactivation", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, false)

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Deactivate(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("deactivate branch with inactive fleet: %v", err)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect branch state: %v", err)
		}
		if active {
			t.Fatal("branch with inactive fleet remained active")
		}
	})

	t.Run("active fleet blocks branch deactivation", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, true)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Deactivate(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasActiveFleets) {
			t.Fatalf(
				"expected ErrBranchHasActiveFleets, got %v",
				err,
			)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect blocked branch: %v", err)
		}
		if !active {
			t.Fatal("blocked branch was deactivated")
		}
	})

	t.Run("inactive non-archived fleet blocks branch archive", func(t *testing.T) {
		f := createFixture(t)
		fleetID := createFleet(t, f, false)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Delete(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasFleets) {
			t.Fatalf("expected ErrBranchHasFleets, got %v", err)
		}

		var branchDeleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&branchDeleted); err != nil {
			t.Fatalf("inspect blocked branch archive: %v", err)
		}
		if branchDeleted {
			t.Fatal("branch archived despite non-archived fleet")
		}

		var fleetDeleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM fleets
			WHERE id=$1
		`, fleetID).Scan(&fleetDeleted); err != nil {
			t.Fatalf("inspect child fleet: %v", err)
		}
		if fleetDeleted {
			t.Fatal("branch archive silently cascaded to child fleet")
		}
	})

	t.Run("active non-archived fleet blocks branch archive", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, true)

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Delete(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasFleets) {
			t.Fatalf("expected ErrBranchHasFleets, got %v", err)
		}
	})

	t.Run("archived fleet does not block branch archive", func(t *testing.T) {
		f := createFixture(t)
		fleetID := createFleet(t, f, false)

		if _, err := db.Exec(ctx, `
			UPDATE fleets
			SET deleted_at=NOW()
			WHERE id=$1
		`, fleetID); err != nil {
			t.Fatalf("archive fixture fleet: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Delete(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("archive branch with only archived fleet: %v", err)
		}

		var deleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&deleted); err != nil {
			t.Fatalf("inspect archived branch: %v", err)
		}
		if !deleted {
			t.Fatal("branch was not archived")
		}
	})

	t.Run("empty branch archives without mutating active state", func(t *testing.T) {
		f := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		if err := service.Delete(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("archive empty branch: %v", err)
		}

		var active bool
		var deleted bool
		if err := db.QueryRow(ctx, `
			SELECT is_active, deleted_at IS NOT NULL
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active, &deleted); err != nil {
			t.Fatalf("inspect archived branch: %v", err)
		}
		if !deleted {
			t.Fatal("branch was not archived")
		}
		if !active {
			t.Fatal("archive silently changed operational active state")
		}
	})

	t.Run("member reactivates inactive branch", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			UPDATE branches
			SET is_active=FALSE
			WHERE id=$1
		`, f.BranchID); err != nil {
			t.Fatalf("deactivate fixture branch: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})
		if err := service.Reactivate(ctx, f.UserID, f.BranchID); err != nil {
			t.Fatalf("reactivate own branch: %v", err)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect reactivated branch: %v", err)
		}
		if !active {
			t.Fatal("branch remained inactive after reactivation")
		}
	})

	t.Run("inactive company blocks branch creation", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			UPDATE companies
			SET is_active=FALSE
			WHERE id=$1
		`, f.CompanyID); err != nil {
			t.Fatalf("deactivate fixture company: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})
		_, err := service.Create(ctx, f.UserID, CreateBranchRequest{
			CompanyID: f.CompanyID,
			Code:      "INACTIVE-" + uuid.NewString()[:8],
			Name:      "Blocked Inactive Company Branch",
		})
		if !errors.Is(err, ErrInvalidCompany) {
			t.Fatalf("expected ErrInvalidCompany, got %v", err)
		}

		var count int
		if err := db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM branches
			WHERE company_id=$1
			  AND id<>$2
		`, f.CompanyID, f.BranchID).Scan(&count); err != nil {
			t.Fatalf("count branches for inactive company: %v", err)
		}
		if count != 0 {
			t.Fatalf("inactive company received %d additional branch(es)", count)
		}
	})

	t.Run("inactive company blocks branch reactivation", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			UPDATE branches
			SET is_active=FALSE
			WHERE id=$1
		`, f.BranchID); err != nil {
			t.Fatalf("deactivate fixture branch: %v", err)
		}

		if _, err := db.Exec(ctx, `
			UPDATE companies
			SET is_active=FALSE
			WHERE id=$1
		`, f.CompanyID); err != nil {
			t.Fatalf("deactivate fixture company: %v", err)
		}

		service := newService([]string{"COMPANY_ADMIN"})
		err := service.Reactivate(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrInvalidCompany) {
			t.Fatalf("expected ErrInvalidCompany, got %v", err)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect blocked branch: %v", err)
		}
		if active {
			t.Fatal("branch became active under an inactive company")
		}
	})

	t.Run("system admin cannot bypass inactive company blocker", func(t *testing.T) {
		f := createFixture(t)

		if _, err := db.Exec(ctx, `
			UPDATE branches
			SET is_active=FALSE
			WHERE id=$1
		`, f.BranchID); err != nil {
			t.Fatalf("deactivate fixture branch: %v", err)
		}

		if _, err := db.Exec(ctx, `
			UPDATE companies
			SET is_active=FALSE
			WHERE id=$1
		`, f.CompanyID); err != nil {
			t.Fatalf("deactivate fixture company: %v", err)
		}

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		err := service.Reactivate(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrInvalidCompany) {
			t.Fatalf(
				"SYSTEM_ADMIN must obey inactive company blocker: %v",
				err,
			)
		}

		var active bool
		if err := db.QueryRow(ctx, `
			SELECT is_active
			FROM branches
			WHERE id=$1
		`, f.BranchID).Scan(&active); err != nil {
			t.Fatalf("inspect blocked branch: %v", err)
		}
		if active {
			t.Fatal("SYSTEM_ADMIN reactivated branch under inactive company")
		}
	})

	t.Run("cross-tenant lifecycle is indistinguishable from nonexistent", func(t *testing.T) {
		owner := createFixture(t)
		other := createFixture(t)
		service := newService([]string{"COMPANY_ADMIN"})

		crossTenantErr := service.Deactivate(
			ctx,
			other.UserID,
			owner.BranchID,
		)
		if !errors.Is(crossTenantErr, repository.ErrNotFound) {
			t.Fatalf(
				"expected repository.ErrNotFound cross-tenant, got %v",
				crossTenantErr,
			)
		}

		missingErr := service.Deactivate(
			ctx,
			other.UserID,
			uuid.NewString(),
		)
		if !errors.Is(missingErr, repository.ErrNotFound) {
			t.Fatalf(
				"expected repository.ErrNotFound nonexistent, got %v",
				missingErr,
			)
		}
	})

	t.Run("system admin is global but cannot bypass active fleet blocker", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, true)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		err := service.Deactivate(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasActiveFleets) {
			t.Fatalf(
				"expected ErrBranchHasActiveFleets, got %v",
				err,
			)
		}
	})

	t.Run("system admin is global but cannot bypass archive blocker", func(t *testing.T) {
		f := createFixture(t)
		createFleet(t, f, false)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		err := service.Delete(ctx, f.UserID, f.BranchID)
		if !errors.Is(err, ErrBranchHasFleets) {
			t.Fatalf(
				"expected ErrBranchHasFleets, got %v",
				err,
			)
		}
	})
}

func TestBranchCreationRechecksCompanyAfterDeactivationLock(t *testing.T) {
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

	userID := uuid.NewString()
	companyID := uuid.NewString()

	if _, err := db.Exec(ctx, `
		INSERT INTO users (
			id, email, password_hash, first_name, last_name
		)
		VALUES ($1, $2, 'test-password-hash', 'Branch', 'CreateRace')
	`, userID, userID+"@example.test"); err != nil {
		t.Fatalf("create concurrency user: %v", err)
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO companies (
			id, name, legal_name, business_id, email,
			country_code, timezone
		)
		VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
	`,
		companyID,
		"Branch Create Race "+companyID[:8],
		"Branch Create Race "+companyID[:8]+" Oy",
		"BCR-"+companyID[:8],
		companyID+"@example.test",
	); err != nil {
		t.Fatalf("create concurrency company: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM branches WHERE company_id=$1`,
			companyID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM companies WHERE id=$1`,
			companyID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id=$1`,
			userID,
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
		"company:"+companyID,
	); err != nil {
		t.Fatalf("acquire controlling company lock: %v", err)
	}

	service := NewService(Dependencies{
		DB:       db,
		Branches: postgresrepo.NewBranchRepository(db),
		UserRoles: &branchAuthorityUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
		CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
	})

	createCtx, cancelCreate := context.WithTimeout(
		ctx,
		5*time.Second,
	)
	defer cancelCreate()

	createResult := make(chan error, 1)
	go func() {
		_, createErr := service.Create(
			createCtx,
			userID,
			CreateBranchRequest{
				CompanyID: companyID,
				Code:      "RACE-" + uuid.NewString()[:8],
				Name:      "Serialized Branch Creation",
			},
		)
		createResult <- createErr
	}()

	var waiting bool
	for attempt := 0; attempt < 100; attempt++ {
		err := db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_locks
				WHERE locktype='advisory'
				  AND NOT granted
			)
		`).Scan(&waiting)
		if err != nil {
			t.Fatalf("inspect advisory lock waiters: %v", err)
		}

		if waiting {
			break
		}

		select {
		case createErr := <-createResult:
			t.Fatalf(
				"branch creation returned before controlling company lock released: %v",
				createErr,
			)
		case <-time.After(10 * time.Millisecond):
		}
	}

	if !waiting {
		t.Fatal("branch creation did not wait on company advisory lock")
	}

	companies := postgresrepo.NewCompanyRepositoryWithDB(blockerTx)
	if err := companies.Deactivate(ctx, companyID); err != nil {
		t.Fatalf("deactivate company in controlling transaction: %v", err)
	}

	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatalf("commit controlling company deactivation: %v", err)
	}
	blockerFinished = true

	select {
	case createErr := <-createResult:
		if !errors.Is(createErr, ErrInvalidCompany) {
			t.Fatalf(
				"expected ErrInvalidCompany after serialized company deactivation, got %v",
				createErr,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("branch creation did not finish after company lock released")
	}

	var count int
	if err := db.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM branches WHERE company_id=$1`,
		companyID,
	).Scan(&count); err != nil {
		t.Fatalf("count branches after rejected creation: %v", err)
	}
	if count != 0 {
		t.Fatalf(
			"expected no branch after serialized company deactivation, found %d",
			count,
		)
	}

	var companyActive bool
	if err := db.QueryRow(
		ctx,
		`SELECT is_active FROM companies WHERE id=$1`,
		companyID,
	).Scan(&companyActive); err != nil {
		t.Fatalf("inspect company after serialized deactivation: %v", err)
	}
	if companyActive {
		t.Fatal("company remained active after controlling deactivation")
	}
}

func TestBranchReactivationRechecksCompanyAfterDeactivationLock(t *testing.T) {
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

	userID := uuid.NewString()
	companyID := uuid.NewString()
	branchID := uuid.NewString()

	if _, err := db.Exec(ctx, `
		INSERT INTO users (
			id, email, password_hash, first_name, last_name
		)
		VALUES ($1, $2, 'test-password-hash', 'Branch', 'ReactivateRace')
	`, userID, userID+"@example.test"); err != nil {
		t.Fatalf("create concurrency user: %v", err)
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO companies (
			id, name, legal_name, business_id, email,
			country_code, timezone
		)
		VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
	`,
		companyID,
		"Branch Reactivate Race "+companyID[:8],
		"Branch Reactivate Race "+companyID[:8]+" Oy",
		"BRR-"+companyID[:8],
		companyID+"@example.test",
	); err != nil {
		t.Fatalf("create concurrency company: %v", err)
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO branches (
			id, company_id, code, name, is_active
		)
		VALUES ($1, $2, $3, $4, FALSE)
	`,
		branchID,
		companyID,
		"BR-"+branchID[:8],
		"Branch Reactivate Race "+branchID[:8],
	); err != nil {
		t.Fatalf("create inactive concurrency branch: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM branches WHERE id=$1`,
			branchID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM companies WHERE id=$1`,
			companyID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id=$1`,
			userID,
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
		"company:"+companyID,
	); err != nil {
		t.Fatalf("acquire controlling company lock: %v", err)
	}

	service := NewService(Dependencies{
		DB:       db,
		Branches: postgresrepo.NewBranchRepository(db),
		UserRoles: &branchAuthorityUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
		CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
	})

	reactivateCtx, cancelReactivate := context.WithTimeout(
		ctx,
		5*time.Second,
	)
	defer cancelReactivate()

	reactivateResult := make(chan error, 1)
	go func() {
		reactivateResult <- service.Reactivate(
			reactivateCtx,
			userID,
			branchID,
		)
	}()

	var waiting bool
	for attempt := 0; attempt < 100; attempt++ {
		if err := db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_locks
				WHERE locktype='advisory'
				  AND NOT granted
			)
		`).Scan(&waiting); err != nil {
			t.Fatalf("inspect advisory lock waiters: %v", err)
		}

		if waiting {
			break
		}

		select {
		case reactivateErr := <-reactivateResult:
			t.Fatalf(
				"branch reactivation returned before controlling company lock released: %v",
				reactivateErr,
			)
		case <-time.After(10 * time.Millisecond):
		}
	}

	if !waiting {
		t.Fatal("branch reactivation did not wait on company advisory lock")
	}

	companies := postgresrepo.NewCompanyRepositoryWithDB(blockerTx)
	if err := companies.Deactivate(ctx, companyID); err != nil {
		t.Fatalf("deactivate company in controlling transaction: %v", err)
	}

	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatalf("commit controlling company deactivation: %v", err)
	}
	blockerFinished = true

	select {
	case reactivateErr := <-reactivateResult:
		if !errors.Is(reactivateErr, ErrInvalidCompany) {
			t.Fatalf(
				"expected ErrInvalidCompany after serialized company deactivation, got %v",
				reactivateErr,
			)
		}
	case <-reactivateCtx.Done():
		t.Fatalf(
			"branch reactivation did not finish after company lock release: %v",
			reactivateCtx.Err(),
		)
	}

	var branchActive bool
	if err := db.QueryRow(
		ctx,
		`SELECT is_active FROM branches WHERE id=$1`,
		branchID,
	).Scan(&branchActive); err != nil {
		t.Fatalf("inspect branch after rejected reactivation: %v", err)
	}
	if branchActive {
		t.Fatal("branch became active under an inactive company")
	}

	var companyActive bool
	if err := db.QueryRow(
		ctx,
		`SELECT is_active FROM companies WHERE id=$1`,
		companyID,
	).Scan(&companyActive); err != nil {
		t.Fatalf("inspect company after serialized deactivation: %v", err)
	}
	if companyActive {
		t.Fatal("company remained active after controlling deactivation")
	}
}
