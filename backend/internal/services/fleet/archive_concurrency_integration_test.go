package fleet_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/models"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	vehicleService "github.com/JCKFinland/connect/backend/internal/services/vehicle"
	"github.com/google/uuid"
)

type concurrencyUserRoleRepositoryStub struct {
	roles []string
}

func (r *concurrencyUserRoleRepositoryStub) AssignRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *concurrencyUserRoleRepositoryStub) RemoveRole(
	context.Context,
	string,
	string,
) error {
	return nil
}

func (r *concurrencyUserRoleRepositoryStub) UserHasRole(
	context.Context,
	string,
	string,
) (bool, error) {
	return false, nil
}

func (r *concurrencyUserRoleRepositoryStub) GetUserRoles(
	context.Context,
	string,
) ([]string, error) {
	return r.roles, nil
}

type concurrencyCompanyMembershipRepositoryStub struct{}

func (r *concurrencyCompanyMembershipRepositoryStub) Create(
	context.Context,
	*models.CompanyMembership,
) error {
	return nil
}

func (r *concurrencyCompanyMembershipRepositoryStub) Exists(
	context.Context,
	string,
	string,
) (bool, error) {
	return true, nil
}

func (r *concurrencyCompanyMembershipRepositoryStub) ListByUserID(
	context.Context,
	string,
) ([]*models.CompanyMembership, error) {
	return nil, nil
}

func TestVehicleCreationRechecksFleetAfterArchiveLock(
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
	t.Cleanup(func() {
		db.Close()
	})

	userID := uuid.NewString()
	companyID := uuid.NewString()
	branchID := uuid.NewString()
	fleetID := uuid.NewString()
	registration := "RACE-" + uuid.NewString()[:8]

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO users (
				id,
				email,
				password_hash,
				first_name,
				last_name
			)
			VALUES ($1, $2, $3, $4, $5)
		`,
		userID,
		userID+"@example.test",
		"test-password-hash",
		"Fleet",
		"Concurrency",
	)
	if err != nil {
		t.Fatalf("create concurrency user: %v", err)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO companies (
				id,
				name,
				legal_name,
				business_id,
				email,
				country_code,
				timezone
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				'FI',
				'Europe/Helsinki'
			)
		`,
		companyID,
		"Fleet Concurrency "+companyID[:8],
		"Fleet Concurrency "+companyID[:8]+" Oy",
		"FC-"+companyID[:8],
		companyID+"@example.test",
	)
	if err != nil {
		t.Fatalf("create concurrency company: %v", err)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO branches (
				id,
				company_id,
				code,
				name
			)
			VALUES ($1, $2, $3, $4)
		`,
		branchID,
		companyID,
		"BR-"+branchID[:8],
		"Fleet Concurrency Branch "+branchID[:8],
	)
	if err != nil {
		t.Fatalf("create concurrency branch: %v", err)
	}

	_, err = db.Exec(
		ctx,
		`
			INSERT INTO fleets (
				id,
				company_id,
				branch_id,
				code,
				name,
				description,
				is_active
			)
			VALUES ($1, $2, $3, $4, $5, $6, TRUE)
		`,
		fleetID,
		companyID,
		branchID,
		"FL-"+fleetID[:8],
		"Fleet Concurrency "+fleetID[:8],
		"fleet archive/create serialization test",
	)
	if err != nil {
		t.Fatalf("create concurrency fleet: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()

		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM vehicles WHERE fleet_id=$1`,
			fleetID,
		)
		_, _ = db.Exec(
			cleanupCtx,
			`DELETE FROM fleets WHERE id=$1`,
			fleetID,
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
		"fleet:"+fleetID,
	); err != nil {
		t.Fatalf("acquire controlling fleet lock: %v", err)
	}

	vehicleSvc := vehicleService.NewService(
		vehicleService.Dependencies{
			DB:       db,
			Vehicles: postgresrepo.NewVehicleRepository(db),
			Fleets:   postgresrepo.NewFleetRepository(db),
			UserRoles: &concurrencyUserRoleRepositoryStub{
				roles: []string{"SYSTEM_ADMIN"},
			},
			CompanyMemberships: &concurrencyCompanyMembershipRepositoryStub{},
		},
	)

	createCtx, cancelCreate := context.WithTimeout(
		ctx,
		5*time.Second,
	)
	defer cancelCreate()

	createResult := make(chan error, 1)

	go func() {
		_, err := vehicleSvc.Create(
			createCtx,
			userID,
			vehicleService.CreateVehicleRequest{
				FleetID:            fleetID,
				RegistrationNumber: registration,
				Make:               "CONNECT",
				Model:              "Concurrency",
				ModelYear:          2026,
				VehicleType:        "SEDAN",
				FuelType:           "ELECTRIC",
				SeatingCapacity:    4,
			},
		)
		createResult <- err
	}()

	// Wait until PostgreSQL reports another backend waiting on this exact
	// advisory lock. This avoids timing-based sleeps and proves Create reached
	// the shared fleet lifecycle serialization boundary.
	var waiting bool
	for attempt := 0; attempt < 100; attempt++ {
		err := db.QueryRow(
			ctx,
			`
				SELECT EXISTS (
					SELECT 1
					FROM pg_locks AS waiting_lock
					WHERE waiting_lock.locktype='advisory'
					  AND NOT waiting_lock.granted
				)
			`,
		).Scan(&waiting)
		if err != nil {
			t.Fatalf(
				"inspect advisory lock waiters: %v",
				err,
			)
		}

		if waiting {
			break
		}

		select {
		case createErr := <-createResult:
			t.Fatalf(
				"vehicle creation returned before controlling lock released: %v",
				createErr,
			)
		case <-time.After(10 * time.Millisecond):
		}
	}

	if !waiting {
		t.Fatal(
			"vehicle creation did not wait on fleet advisory lock",
		)
	}

	// Force the archive-wins ordering while still holding the exact same
	// lifecycle lock. The waiting Create transaction cannot inspect the fleet
	// until this transaction commits.
	fleets := postgresrepo.NewFleetRepositoryWithDB(
		blockerTx,
	)
	if err := fleets.Archive(
		ctx,
		fleetID,
	); err != nil {
		t.Fatalf(
			"archive fleet in controlling transaction: %v",
			err,
		)
	}

	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatalf(
			"commit controlling fleet archive: %v",
			err,
		)
	}
	blockerFinished = true

	select {
	case createErr := <-createResult:
		if !errors.Is(
			createErr,
			vehicleService.ErrInvalidFleet,
		) {
			t.Fatalf(
				"expected ErrInvalidFleet after serialized archive, got %v",
				createErr,
			)
		}
	case <-createCtx.Done():
		t.Fatalf(
			"vehicle creation did not finish after fleet lock release: %v",
			createCtx.Err(),
		)
	}

	var vehicleCount int
	if err := db.QueryRow(
		ctx,
		`
			SELECT COUNT(*)
			FROM vehicles
			WHERE fleet_id=$1
			  AND registration_number=$2
			  AND deleted_at IS NULL
		`,
		fleetID,
		registration,
	).Scan(&vehicleCount); err != nil {
		t.Fatalf(
			"count concurrent vehicle inserts: %v",
			err,
		)
	}

	if vehicleCount != 0 {
		t.Fatalf(
			"archived fleet gained %d non-archived vehicle(s)",
			vehicleCount,
		)
	}

	var fleetArchived bool
	if err := db.QueryRow(
		ctx,
		`
			SELECT deleted_at IS NOT NULL
			FROM fleets
			WHERE id=$1
		`,
		fleetID,
	).Scan(&fleetArchived); err != nil {
		t.Fatalf(
			"inspect fleet after concurrent create: %v",
			err,
		)
	}

	if !fleetArchived {
		t.Fatal(
			"controlling fleet archive was not preserved",
		)
	}
}
