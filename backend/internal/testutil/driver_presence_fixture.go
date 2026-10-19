package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateDriverPresenceFixture creates an online, available presence
// record for a disposable driver hierarchy.
//
// driver_presence.driver_id references users.id, not drivers.id.
//
// Call cleanup before cleaning up the owning DriverFixture.
func CreateDriverPresenceFixture(
	ctx context.Context,
	db *pgxpool.Pool,
	fixture *DriverFixture,
) (func(context.Context) error, error) {
	if db == nil {
		return nil, fmt.Errorf("database pool is required")
	}

	if fixture == nil {
		return nil, fmt.Errorf("driver fixture is required")
	}

	if fixture.UserID == "" ||
		fixture.CompanyID == "" ||
		fixture.BranchID == "" ||
		fixture.VehicleID == "" ||
		fixture.AssignmentID == "" {
		return nil, fmt.Errorf(
			"driver fixture has incomplete presence relationships",
		)
	}

	_, err := db.Exec(
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
            latitude,
            longitude,
            last_heartbeat_at
        )
        VALUES (
            $1, $2, $3, $4, $5,
            TRUE, 'AVAILABLE',
            60.2055, 24.6559, $6
        )
        `,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.VehicleID,
		fixture.AssignmentID,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create isolated driver presence: %w",
			err,
		)
	}

	cleanup := func(cleanupCtx context.Context) error {
		_, err := db.Exec(
			cleanupCtx,
			`
            DELETE FROM driver_presence
            WHERE driver_id = $1
            `,
			fixture.UserID,
		)
		if err != nil {
			return fmt.Errorf(
				"delete isolated driver presence: %w",
				err,
			)
		}

		return nil
	}

	return cleanup, nil
}
