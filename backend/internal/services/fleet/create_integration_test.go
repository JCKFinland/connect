package fleet

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

type fleetCreateIntegrationFixture struct {
	UserID    string
	CompanyID string
	BranchID  string
}

func TestFleetCreateEnforcesTenantAndBranchLifecycle(t *testing.T) {
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

	createFixture := func(t *testing.T, branchActive bool) fleetCreateIntegrationFixture {
		t.Helper()

		f := fleetCreateIntegrationFixture{
			UserID:    uuid.NewString(),
			CompanyID: uuid.NewString(),
			BranchID:  uuid.NewString(),
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO users (
				id, email, password_hash, first_name, last_name
			)
			VALUES ($1, $2, 'test-password-hash', 'Fleet', 'Create')
		`, f.UserID, f.UserID+"@example.test"); err != nil {
			t.Fatalf("create fleet creation user: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO companies (
				id, name, legal_name, business_id, email,
				country_code, timezone
			)
			VALUES ($1, $2, $3, $4, $5, 'FI', 'Europe/Helsinki')
		`,
			f.CompanyID,
			"Fleet Create "+f.CompanyID[:8],
			"Fleet Create "+f.CompanyID[:8]+" Oy",
			"FCR-"+f.CompanyID[:8],
			f.CompanyID+"@example.test",
		); err != nil {
			t.Fatalf("create fleet creation company: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO branches (
				id,
				company_id,
				code,
				name,
				email,
				phone,
				address_line1,
				address_line2,
				city,
				state,
				postal_code,
				latitude,
				longitude,
				is_active
			)
			VALUES (
				$1, $2, $3, $4, $5, $6, $7,
				$8, $9, $10, $11, $12, $13, $14
			)
		`,
			f.BranchID,
			f.CompanyID,
			"BR-"+f.BranchID[:8],
			"Fleet Create Branch "+f.BranchID[:8],
			f.BranchID+"@example.test",
			"+358401234567",
			"1 Testikatu",
			"",
			"Helsinki",
			"Uusimaa",
			"00100",
			60.1699,
			24.9384,
			branchActive,
		); err != nil {
			t.Fatalf("create fleet creation branch: %v", err)
		}

		if _, err := db.Exec(ctx, `
			INSERT INTO company_memberships (user_id, company_id)
			VALUES ($1, $2)
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("create fleet creation membership: %v", err)
		}

		t.Cleanup(func() {
			cleanupCtx := context.Background()
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM fleets WHERE branch_id=$1`, f.BranchID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM company_memberships WHERE user_id=$1`, f.UserID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM branches WHERE id=$1`, f.BranchID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM companies WHERE id=$1`, f.CompanyID)
			_, _ = db.Exec(cleanupCtx,
				`DELETE FROM users WHERE id=$1`, f.UserID)
		})

		return f
	}

	newService := func(roles []string) *Service {
		return NewService(Dependencies{
			DB:     db,
			Fleets: postgresrepo.NewFleetRepository(db),
			UserRoles: &fleetCreateUserRoleRepositoryStub{
				roles: roles,
			},
		})
	}

	t.Run("member creates active fleet with company derived from branch", func(t *testing.T) {
		f := createFixture(t, true)
		service := newService([]string{"COMPANY_ADMIN"})

		result, err := service.Create(ctx, f.UserID, CreateFleetRequest{
			BranchID:    f.BranchID,
			Code:        "FLEET-" + uuid.NewString()[:8],
			Name:        "Main Fleet",
			Description: "Primary fleet",
		})
		if err != nil {
			t.Fatalf("create own fleet: %v", err)
		}

		if result.CompanyID != f.CompanyID {
			t.Fatalf(
				"expected company derived from branch %q, got %q",
				f.CompanyID,
				result.CompanyID,
			)
		}
		if result.BranchID != f.BranchID {
			t.Fatalf(
				"expected authoritative branch %q, got %q",
				f.BranchID,
				result.BranchID,
			)
		}
		if !result.IsActive {
			t.Fatal("expected service-owned initial active state")
		}

		var companyID, branchID string
		var active bool
		if err := db.QueryRow(ctx, `
			SELECT company_id, branch_id, is_active
			FROM fleets
			WHERE id=$1
		`, result.ID).Scan(&companyID, &branchID, &active); err != nil {
			t.Fatalf("inspect created fleet: %v", err)
		}
		if companyID != f.CompanyID || branchID != f.BranchID || !active {
			t.Fatalf(
				"unexpected persisted authority fields: company=%q branch=%q active=%v",
				companyID,
				branchID,
				active,
			)
		}
	})

	t.Run("cross-tenant branch is indistinguishable from nonexistent", func(t *testing.T) {
		owner := createFixture(t, true)
		other := createFixture(t, true)
		service := newService([]string{"COMPANY_ADMIN"})

		_, crossTenantErr := service.Create(ctx, other.UserID, CreateFleetRequest{
			BranchID: owner.BranchID,
			Code:     "CROSS-" + uuid.NewString()[:8],
			Name:     "Cross Tenant Fleet",
		})
		if !errors.Is(crossTenantErr, ErrInvalidBranch) {
			t.Fatalf(
				"expected ErrInvalidBranch cross-tenant, got %v",
				crossTenantErr,
			)
		}

		_, missingErr := service.Create(ctx, other.UserID, CreateFleetRequest{
			BranchID: uuid.NewString(),
			Code:     "MISS-" + uuid.NewString()[:8],
			Name:     "Missing Branch Fleet",
		})
		if !errors.Is(missingErr, ErrInvalidBranch) {
			t.Fatalf(
				"expected ErrInvalidBranch nonexistent, got %v",
				missingErr,
			)
		}
	})

	t.Run("inactive branch blocks creation", func(t *testing.T) {
		f := createFixture(t, false)
		service := newService([]string{"COMPANY_ADMIN"})

		_, err := service.Create(ctx, f.UserID, CreateFleetRequest{
			BranchID: f.BranchID,
			Code:     "INACTIVE-" + uuid.NewString()[:8],
			Name:     "Inactive Branch Fleet",
		})
		if !errors.Is(err, ErrInvalidBranch) {
			t.Fatalf("expected ErrInvalidBranch, got %v", err)
		}

		var count int
		if err := db.QueryRow(ctx,
			`SELECT COUNT(*) FROM fleets WHERE branch_id=$1`,
			f.BranchID,
		).Scan(&count); err != nil {
			t.Fatalf("count fleets for inactive branch: %v", err)
		}
		if count != 0 {
			t.Fatalf("inactive branch received %d fleet(s)", count)
		}
	})

	t.Run("system admin is global without company membership", func(t *testing.T) {
		f := createFixture(t, true)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		result, err := service.Create(ctx, f.UserID, CreateFleetRequest{
			BranchID: f.BranchID,
			Code:     "ADMIN-" + uuid.NewString()[:8],
			Name:     "System Admin Fleet",
		})
		if err != nil {
			t.Fatalf("SYSTEM_ADMIN global fleet creation: %v", err)
		}

		if result.CompanyID != f.CompanyID ||
			result.BranchID != f.BranchID ||
			!result.IsActive {
			t.Fatalf("unexpected SYSTEM_ADMIN result: %#v", result)
		}
	})

	t.Run("system admin still obeys inactive branch blocker", func(t *testing.T) {
		f := createFixture(t, false)

		if _, err := db.Exec(ctx, `
			DELETE FROM company_memberships
			WHERE user_id=$1 AND company_id=$2
		`, f.UserID, f.CompanyID); err != nil {
			t.Fatalf("remove system admin membership: %v", err)
		}

		service := newService([]string{"SYSTEM_ADMIN"})
		_, err := service.Create(ctx, f.UserID, CreateFleetRequest{
			BranchID: f.BranchID,
			Code:     "ADMIN-INACTIVE-" + uuid.NewString()[:8],
			Name:     "Blocked System Admin Fleet",
		})
		if !errors.Is(err, ErrInvalidBranch) {
			t.Fatalf(
				"SYSTEM_ADMIN must obey inactive branch blocker: %v",
				err,
			)
		}
	})
}

func TestFleetCreationRechecksBranchAfterDeactivationLock(t *testing.T) {
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
		VALUES ($1, $2, 'test-password-hash', 'Fleet', 'CreateRace')
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
		"Fleet Create Race "+companyID[:8],
		"Fleet Create Race "+companyID[:8]+" Oy",
		"FCR-"+companyID[:8],
		companyID+"@example.test",
	); err != nil {
		t.Fatalf("create concurrency company: %v", err)
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO branches (
			id,
			company_id,
			code,
			name,
			email,
			phone,
			address_line1,
			address_line2,
			city,
			state,
			postal_code,
			latitude,
			longitude,
			is_active
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, TRUE
		)
	`,
		branchID,
		companyID,
		"BR-"+branchID[:8],
		"Fleet Create Race Branch "+branchID[:8],
		branchID+"@example.test",
		"+358401234567",
		"1 Testikatu",
		"",
		"Helsinki",
		"Uusimaa",
		"00100",
		60.1699,
		24.9384,
	); err != nil {
		t.Fatalf("create concurrency branch: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(cleanupCtx, `DELETE FROM fleets WHERE branch_id=$1`, branchID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM branches WHERE id=$1`, branchID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM companies WHERE id=$1`, companyID)
		_, _ = db.Exec(cleanupCtx, `DELETE FROM users WHERE id=$1`, userID)
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
		"branch:"+branchID,
	); err != nil {
		t.Fatalf("acquire controlling branch lock: %v", err)
	}

	service := NewService(Dependencies{
		DB:     db,
		Fleets: postgresrepo.NewFleetRepository(db),
		UserRoles: &fleetCreateUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	createCtx, cancelCreate := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCreate()

	createResult := make(chan error, 1)
	go func() {
		_, createErr := service.Create(
			createCtx,
			userID,
			CreateFleetRequest{
				BranchID: branchID,
				Code:     "RACE-" + uuid.NewString()[:8],
				Name:     "Serialized Fleet Creation",
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
				"fleet creation returned before controlling branch lock released: %v",
				createErr,
			)
		case <-time.After(10 * time.Millisecond):
		}
	}

	if !waiting {
		t.Fatal("fleet creation did not wait on branch advisory lock")
	}

	branches := postgresrepo.NewBranchRepositoryWithDB(blockerTx)
	if err := branches.Deactivate(ctx, branchID); err != nil {
		t.Fatalf("deactivate branch in controlling transaction: %v", err)
	}

	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatalf("commit controlling branch deactivation: %v", err)
	}
	blockerFinished = true

	select {
	case createErr := <-createResult:
		if !errors.Is(createErr, ErrInvalidBranch) {
			t.Fatalf(
				"expected ErrInvalidBranch after serialized branch deactivation, got %v",
				createErr,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fleet creation did not finish after branch lock released")
	}

	var count int
	if err := db.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM fleets WHERE branch_id=$1`,
		branchID,
	).Scan(&count); err != nil {
		t.Fatalf("count fleets after rejected creation: %v", err)
	}
	if count != 0 {
		t.Fatalf(
			"expected no fleet after serialized branch deactivation, found %d",
			count,
		)
	}

	var branchActive bool
	if err := db.QueryRow(
		ctx,
		`SELECT is_active FROM branches WHERE id=$1`,
		branchID,
	).Scan(&branchActive); err != nil {
		t.Fatalf("inspect branch after serialized deactivation: %v", err)
	}
	if branchActive {
		t.Fatal("branch remained active after controlling deactivation")
	}
}

func TestFleetCreationRechecksBranchAfterArchiveLock(t *testing.T) {
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
		VALUES ($1, $2, 'test-password-hash', 'Fleet', 'ArchiveRace')
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
		"Fleet Archive Race "+companyID[:8],
		"Fleet Archive Race "+companyID[:8]+" Oy",
		"FAR-"+companyID[:8],
		companyID+"@example.test",
	); err != nil {
		t.Fatalf("create concurrency company: %v", err)
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO branches (
			id,
			company_id,
			code,
			name,
			email,
			phone,
			address_line1,
			address_line2,
			city,
			state,
			postal_code,
			latitude,
			longitude,
			is_active
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, TRUE
		)
	`,
		branchID,
		companyID,
		"BR-"+branchID[:8],
		"Fleet Archive Race Branch "+branchID[:8],
		branchID+"@example.test",
		"+358401234567",
		"1 Testikatu",
		"",
		"Helsinki",
		"Uusimaa",
		"00100",
		60.1699,
		24.9384,
	); err != nil {
		t.Fatalf("create concurrency branch: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM fleets WHERE branch_id=$1`,
			branchID,
		)
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
		"branch:"+branchID,
	); err != nil {
		t.Fatalf("acquire controlling branch lock: %v", err)
	}

	service := NewService(Dependencies{
		DB:     db,
		Fleets: postgresrepo.NewFleetRepository(db),
		UserRoles: &fleetCreateUserRoleRepositoryStub{
			roles: []string{"SYSTEM_ADMIN"},
		},
	})

	createCtx, cancelCreate := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCreate()

	createResult := make(chan error, 1)
	go func() {
		_, createErr := service.Create(
			createCtx,
			userID,
			CreateFleetRequest{
				BranchID: branchID,
				Code:     "ARCHIVE-RACE-" + uuid.NewString()[:8],
				Name:     "Serialized Fleet Creation After Archive",
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
				"fleet creation returned before controlling branch lock released: %v",
				createErr,
			)
		case <-time.After(10 * time.Millisecond):
		}
	}

	if !waiting {
		t.Fatal("fleet creation did not wait on branch advisory lock")
	}

	branches := postgresrepo.NewBranchRepositoryWithDB(blockerTx)
	if err := branches.Archive(ctx, branchID); err != nil {
		t.Fatalf("archive branch in controlling transaction: %v", err)
	}

	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatalf("commit controlling branch archive: %v", err)
	}
	blockerFinished = true

	select {
	case createErr := <-createResult:
		if !errors.Is(createErr, ErrInvalidBranch) {
			t.Fatalf(
				"expected ErrInvalidBranch after serialized branch archive, got %v",
				createErr,
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fleet creation did not finish after branch lock released")
	}

	var count int
	if err := db.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM fleets WHERE branch_id=$1`,
		branchID,
	).Scan(&count); err != nil {
		t.Fatalf("count fleets after rejected creation: %v", err)
	}
	if count != 0 {
		t.Fatalf(
			"expected no fleet after serialized branch archive, found %d",
			count,
		)
	}

	var archived bool
	if err := db.QueryRow(
		ctx,
		`SELECT deleted_at IS NOT NULL FROM branches WHERE id=$1`,
		branchID,
	).Scan(&archived); err != nil {
		t.Fatalf("inspect branch after serialized archive: %v", err)
	}
	if !archived {
		t.Fatal("branch was not archived by controlling transaction")
	}
}
