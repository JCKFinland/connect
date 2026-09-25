package driver

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestVerifyActivatesDriverAndAssignsDriverRole(t *testing.T) {
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
	defer db.Close()

	fixture, cleanup, err := createVerificationFixture(ctx, db)
	if err != nil {
		t.Fatalf("create verification fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup verification fixture: %v", err)
		}
	}()

	driverRepo := postgresrepo.NewDriverRepository(db)

	service := NewService(Dependencies{
		DB:      db,
		Drivers: driverRepo,
	})

	result, err := service.Verify(
		ctx,
		fixture.DriverID,
		fixture.VerifierID,
	)
	if err != nil {
		t.Fatalf("verify driver: %v", err)
	}

	if result.Status != "ACTIVE" {
		t.Fatalf("expected ACTIVE, got %s", result.Status)
	}

	if !result.IsVerified {
		t.Fatal("expected driver to be verified")
	}

	if result.VerifiedAt == nil {
		t.Fatal("expected verified_at")
	}

	if result.VerifiedByUserID == nil {
		t.Fatal("expected verified_by_user_id")
	}

	if *result.VerifiedByUserID != fixture.VerifierID {
		t.Fatalf(
			"expected verifier %s, got %s",
			fixture.VerifierID,
			*result.VerifiedByUserID,
		)
	}

	persisted, err := driverRepo.GetByID(ctx, fixture.DriverID)
	if err != nil {
		t.Fatalf("reload verified driver: %v", err)
	}

	if persisted.Status != "ACTIVE" || !persisted.IsVerified {
		t.Fatalf(
			"expected persisted ACTIVE verified driver, got status=%s verified=%v",
			persisted.Status,
			persisted.IsVerified,
		)
	}

	if persisted.VerifiedAt == nil ||
		persisted.VerifiedByUserID == nil ||
		*persisted.VerifiedByUserID != fixture.VerifierID {
		t.Fatal("expected persisted verification audit metadata")
	}

	roleRepo := repository.NewRoleRepository(db)

	driverRole, err := roleRepo.GetByName(ctx, "DRIVER")
	if err != nil {
		t.Fatalf("get DRIVER role: %v", err)
	}

	userRoleRepo := repository.NewUserRoleRepository(db)

	hasDriverRole, err := userRoleRepo.UserHasRole(
		ctx,
		fixture.UserID,
		driverRole.ID,
	)
	if err != nil {
		t.Fatalf("check DRIVER role assignment: %v", err)
	}

	if !hasDriverRole {
		t.Fatal("expected verified user to have DRIVER role")
	}
}

func TestVerifyRejectsDriverNotPendingVerification(t *testing.T) {
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
	defer db.Close()

	fixture, cleanup, err := createVerificationFixture(ctx, db)
	if err != nil {
		t.Fatalf("create verification fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup verification fixture: %v", err)
		}
	}()

	service := NewService(Dependencies{
		DB:      db,
		Drivers: postgresrepo.NewDriverRepository(db),
	})

	_, err = service.Verify(
		ctx,
		fixture.DriverID,
		fixture.VerifierID,
	)
	if err != nil {
		t.Fatalf("first verification: %v", err)
	}

	_, err = service.Verify(
		ctx,
		fixture.DriverID,
		fixture.VerifierID,
	)
	if !errors.Is(err, ErrDriverNotPendingVerification) {
		t.Fatalf(
			"expected ErrDriverNotPendingVerification, got %v",
			err,
		)
	}
}

func TestVerifyRejectsExpiredDrivingLicenseWithoutGrantingRole(
	t *testing.T,
) {
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
	defer db.Close()

	fixture, cleanup, err := createVerificationFixture(ctx, db)
	if err != nil {
		t.Fatalf("create verification fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup verification fixture: %v", err)
		}
	}()

	_, err = db.Exec(
		ctx,
		`
			UPDATE drivers
			SET driving_license_expiry = CURRENT_DATE - 1
			WHERE id = $1
		`,
		fixture.DriverID,
	)
	if err != nil {
		t.Fatalf("expire driving licence: %v", err)
	}

	driverRepo := postgresrepo.NewDriverRepository(db)

	service := NewService(Dependencies{
		DB:      db,
		Drivers: driverRepo,
	})

	_, err = service.Verify(
		ctx,
		fixture.DriverID,
		fixture.VerifierID,
	)
	if !errors.Is(err, ErrInvalidDriver) {
		t.Fatalf("expected ErrInvalidDriver, got %v", err)
	}

	persisted, err := driverRepo.GetByID(ctx, fixture.DriverID)
	if err != nil {
		t.Fatalf("reload driver: %v", err)
	}

	if persisted.Status != "PENDING_VERIFICATION" {
		t.Fatalf(
			"expected PENDING_VERIFICATION, got %s",
			persisted.Status,
		)
	}

	if persisted.IsVerified {
		t.Fatal("expected driver to remain unverified")
	}

	if persisted.VerifiedAt != nil {
		t.Fatal("expected verified_at to remain nil")
	}

	if persisted.VerifiedByUserID != nil {
		t.Fatal("expected verified_by_user_id to remain nil")
	}

	roleRepo := repository.NewRoleRepository(db)

	driverRole, err := roleRepo.GetByName(ctx, "DRIVER")
	if err != nil {
		t.Fatalf("get DRIVER role: %v", err)
	}

	userRoleRepo := repository.NewUserRoleRepository(db)

	hasDriverRole, err := userRoleRepo.UserHasRole(
		ctx,
		fixture.UserID,
		driverRole.ID,
	)
	if err != nil {
		t.Fatalf("check DRIVER role: %v", err)
	}

	if hasDriverRole {
		t.Fatal("expired-licence applicant must not receive DRIVER role")
	}
}

func TestVerifyReturnsDriverNotFound(t *testing.T) {
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
	defer db.Close()

	service := NewService(Dependencies{
		DB:      db,
		Drivers: postgresrepo.NewDriverRepository(db),
	})

	_, err = service.Verify(
		ctx,
		uuid.NewString(),
		uuid.NewString(),
	)
	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf(
			"expected ErrDriverNotFound, got %v",
			err,
		)
	}
}

func TestVerifySerializesConcurrentAttempts(t *testing.T) {
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
	defer db.Close()

	fixture, cleanup, err := createVerificationFixture(ctx, db)
	if err != nil {
		t.Fatalf("create verification fixture: %v", err)
	}
	defer func() {
		if err := cleanup(context.Background()); err != nil {
			t.Logf("cleanup verification fixture: %v", err)
		}
	}()

	service := NewService(Dependencies{
		DB:      db,
		Drivers: postgresrepo.NewDriverRepository(db),
	})

	start := make(chan struct{})
	results := make(chan error, 2)

	verify := func() {
		<-start

		_, err := service.Verify(
			ctx,
			fixture.DriverID,
			fixture.VerifierID,
		)

		results <- err
	}

	go verify()
	go verify()

	close(start)

	err1 := <-results
	err2 := <-results

	successes := 0
	notPending := 0

	for _, err := range []error{err1, err2} {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrDriverNotPendingVerification):
			notPending++
		default:
			t.Fatalf("unexpected verification result: %v", err)
		}
	}

	if successes != 1 {
		t.Fatalf(
			"expected exactly one successful verification, got %d",
			successes,
		)
	}

	if notPending != 1 {
		t.Fatalf(
			"expected exactly one non-pending result, got %d",
			notPending,
		)
	}

	var roleCount int

	err = db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1
			  AND r.name = 'DRIVER'
		`,
		fixture.UserID,
	).Scan(&roleCount)
	if err != nil {
		t.Fatalf("count DRIVER role assignments: %v", err)
	}

	if roleCount != 1 {
		t.Fatalf(
			"expected exactly one DRIVER role assignment, got %d",
			roleCount,
		)
	}
}
