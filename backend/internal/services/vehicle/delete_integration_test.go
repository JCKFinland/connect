package vehicle

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
)

func TestDeleteRejectsVehicleWithActiveAssignment(
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

	fixture, cleanup, err :=
		testutil.CreateDriverFixture(
			ctx,
			db,
		)
	if err != nil {
		t.Fatalf("create driver fixture: %v", err)
	}

	t.Cleanup(func() {
		if err := cleanup(context.Background()); err != nil {
			t.Errorf("cleanup driver fixture: %v", err)
		}
	})

	roleRepo := repository.NewRoleRepository(db)

	systemAdminRole, err :=
		roleRepo.GetByName(
			ctx,
			"SYSTEM_ADMIN",
		)
	if err != nil {
		t.Fatalf("resolve SYSTEM_ADMIN role: %v", err)
	}

	userRoleRepo :=
		repository.NewUserRoleRepository(db)

	if err := userRoleRepo.AssignRole(
		ctx,
		fixture.UserID,
		systemAdminRole.ID,
	); err != nil {
		t.Fatalf(
			"grant SYSTEM_ADMIN role to fixture user: %v",
			err,
		)
	}

	service := NewService(
		Dependencies{
			DB:                 db,
			Vehicles:           postgresrepo.NewVehicleRepository(db),
			UserRoles:          userRoleRepo,
			CompanyMemberships: postgresrepo.NewCompanyMembershipRepository(db),
		},
	)

	err = service.Delete(
		ctx,
		fixture.UserID,
		fixture.VehicleID,
	)
	if !errors.Is(
		err,
		ErrVehicleHasActiveAssignment,
	) {
		t.Fatalf(
			"expected ErrVehicleHasActiveAssignment, got %v",
			err,
		)
	}

	var (
		archived bool
		active   bool
	)

	err = db.QueryRow(
		ctx,
		`
			SELECT
				deleted_at IS NOT NULL,
				is_active
			FROM vehicles
			WHERE id = $1
		`,
		fixture.VehicleID,
	).Scan(
		&archived,
		&active,
	)
	if err != nil {
		t.Fatalf(
			"inspect vehicle after rejected archive: %v",
			err,
		)
	}

	if archived {
		t.Fatal(
			"active-assignment rejection must leave vehicle unarchived",
		)
	}

	if !active {
		t.Fatal(
			"rejected archive must not mutate operational is_active authority",
		)
	}
}
