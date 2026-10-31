package driver_payout

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recoveryOrganizationFixture struct {
	BranchID  string
	FleetID   string
	VehicleID string
}

func newRecoveryOrganizationFixture(
	t *testing.T,
	db *pgxpool.Pool,
	companyID string,
) *recoveryOrganizationFixture {
	t.Helper()

	ctx := context.Background()

	var databaseName string
	var migration int

	err := db.QueryRow(ctx, `
        SELECT current_database(),
               COALESCE(
                   (SELECT MAX(version_id)
                    FROM goose_db_version
                    WHERE is_applied=true), 0)
    `).Scan(&databaseName, &migration)

	if err != nil ||
		databaseName != "connect_payout_destination_test" ||
		migration != 75 {
		t.Fatalf(
			"unsafe organization fixture database: %s migration=%d err=%v",
			databaseName, migration, err,
		)
	}

	if uuid.Validate(companyID) != nil {
		t.Fatal("invalid payout company ID")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())

	suffix := uuid.NewString()[:8]

	fixture := &recoveryOrganizationFixture{}

	err = tx.QueryRow(ctx, `
        INSERT INTO branches (
            company_id, code, name, city, is_active
        )
        VALUES ($1::uuid, $2, $3, 'Espoo', TRUE)
        RETURNING id::text
    `,
		companyID,
		"REC-"+suffix,
		"Recovery Test Branch "+suffix,
	).Scan(&fixture.BranchID)

	if err != nil {
		t.Fatalf("create recovery branch: %v", err)
	}

	err = tx.QueryRow(ctx, `
        INSERT INTO fleets (
            company_id, branch_id, code, name, is_active
        )
        VALUES ($1::uuid, $2::uuid, $3, $4, TRUE)
        RETURNING id::text
    `,
		companyID,
		fixture.BranchID,
		"REC-"+suffix,
		"Recovery Test Fleet "+suffix,
	).Scan(&fixture.FleetID)

	if err != nil {
		t.Fatalf("create recovery fleet: %v", err)
	}

	err = tx.QueryRow(ctx, `
        INSERT INTO vehicles (
            company_id,
            branch_id,
            fleet_id,
            registration_number,
            make,
            model,
            vehicle_type,
            seating_capacity,
            is_active
        )
        VALUES (
            $1::uuid, $2::uuid, $3::uuid,
            $4, 'CONNECT', 'Recovery Test Vehicle',
            'SEDAN', 4, TRUE
        )
        RETURNING id::text
    `,
		companyID,
		fixture.BranchID,
		fixture.FleetID,
		"REC-"+suffix,
	).Scan(&fixture.VehicleID)

	if err != nil {
		t.Fatalf("create recovery vehicle: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit recovery organization fixture: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer cancel()

		cleanupTx, err := db.Begin(cleanupCtx)
		if err != nil {
			t.Errorf("begin organization cleanup: %v", err)
			return
		}
		defer cleanupTx.Rollback(context.Background())

		operations := []struct {
			table string
			id    string
		}{
			{"vehicles", fixture.VehicleID},
			{"fleets", fixture.FleetID},
			{"branches", fixture.BranchID},
		}

		for _, op := range operations {
			var statement string

			switch op.table {
			case "vehicles":
				statement = `DELETE FROM vehicles WHERE id=$1::uuid`
			case "fleets":
				statement = `DELETE FROM fleets WHERE id=$1::uuid`
			case "branches":
				statement = `DELETE FROM branches WHERE id=$1::uuid`
			}

			if _, err := cleanupTx.Exec(
				cleanupCtx, statement, op.id,
			); err != nil {
				t.Errorf(
					"cleanup %s %s: %v",
					op.table, op.id, err,
				)
				return
			}
		}

		if err := cleanupTx.Commit(cleanupCtx); err != nil {
			t.Errorf("commit organization cleanup: %v", err)
		}
	})

	return fixture
}
