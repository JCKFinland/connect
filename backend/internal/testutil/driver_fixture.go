package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DriverFixture owns a complete disposable driver hierarchy for integration
// tests. Every created row belongs to the fixture and can be safely removed
// without modifying shared/live development data.
type DriverFixture struct {
	CompanyID    string
	BranchID     string
	FleetID      string
	VehicleID    string
	UserID       string
	DriverID     string
	AssignmentID string
}

// CreateDriverFixture creates an isolated company, branch, fleet, vehicle,
// user, driver, and active driver assignment.
//
// The returned cleanup function must always be called.
func CreateDriverFixture(
	ctx context.Context,
	db *pgxpool.Pool,
) (*DriverFixture, func(context.Context) error, error) {

	if db == nil {
		return nil, nil, fmt.Errorf("database pool is required")
	}

	fixture := &DriverFixture{}

	fixtureKey := uuid.NewString()
	shortKey := fixtureKey[:8]
	now := time.Now().UTC()

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"begin driver fixture transaction: %w",
			err,
		)
	}

	rollback := func() {
		_ = tx.Rollback(context.Background())
	}

	// Company.
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO companies (
				name,
				legal_name,
				business_id,
				email,
				phone,
				country_code,
				timezone,
				city,
				is_active
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				'FI',
				'Europe/Helsinki',
				'Espoo',
				TRUE
			)
			RETURNING id
		`,
		"CONNECT Test Company "+shortKey,
		"CONNECT Test Company "+shortKey,
		"TEST-"+fixtureKey,
		"company-"+fixtureKey+"@example.test",
		"+358000000001",
	).Scan(&fixture.CompanyID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf(
			"create test company: %w",
			err,
		)
	}

	// Branch.
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO branches (
				company_id,
				code,
				name,
				email,
				phone,
				city,
				latitude,
				longitude,
				is_active
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				'Espoo',
				60.2055,
				24.6559,
				TRUE
			)
			RETURNING id
		`,
		fixture.CompanyID,
		"TEST-"+shortKey,
		"CONNECT Test Branch "+shortKey,
		"branch-"+fixtureKey+"@example.test",
		"+358000000002",
	).Scan(&fixture.BranchID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf(
			"create test branch: %w",
			err,
		)
	}

	// Fleet.
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO fleets (
				company_id,
				branch_id,
				code,
				name,
				description,
				is_active
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				'Disposable CONNECT integration-test fleet',
				TRUE
			)
			RETURNING id
		`,
		fixture.CompanyID,
		fixture.BranchID,
		"TEST-"+shortKey,
		"CONNECT Test Fleet "+shortKey,
	).Scan(&fixture.FleetID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf(
			"create test fleet: %w",
			err,
		)
	}

	// Vehicle.
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO vehicles (
				company_id,
				branch_id,
				fleet_id,
				registration_number,
				vin,
				make,
				model,
				model_year,
				color,
				vehicle_type,
				seating_capacity,
				is_active
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				'CONNECT',
				'Test Vehicle',
				2026,
				'Black',
				'SEDAN',
				4,
				TRUE
			)
			RETURNING id
		`,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.FleetID,
		"TST-"+shortKey,
		"VIN"+shortKey,
	).Scan(&fixture.VehicleID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf(
			"create test vehicle: %w",
			err,
		)
	}

	// User.
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO users (
				email,
				password_hash,
				first_name,
				last_name,
				phone
			)
			VALUES (
				$1,
				$2,
				'Integration',
				'Driver',
				$3
			)
			RETURNING id
		`,
		"driver-"+fixtureKey+"@example.test",
		"test-fixture-password-hash",
		"+358000000003",
	).Scan(&fixture.UserID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf(
			"create test driver user: %w",
			err,
		)
	}

	// Driver.
	fixture.DriverID = uuid.NewString()

	_, err = tx.Exec(
		ctx,
		`
			INSERT INTO drivers (
				id,
				user_id,
				company_id,
				branch_id,
				driver_number,
				first_name,
				last_name,
				phone,
				email,
				taxi_driver_license_number,
				driving_license_number,
				driving_license_expiry,
				hire_date,
				status,
				is_verified,
				is_active,
				created_at,
				updated_at
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				'Integration',
				'Driver',
				$6,
				$7,
				$8,
				$9,
				$10,
				$11,
				'ACTIVE',
				TRUE,
				TRUE,
				$12,
				$12
			)
		`,
		fixture.DriverID,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		"DRV-"+fixtureKey,
		"+358000000003",
		"driver-"+fixtureKey+"@example.test",
		"TAXI-"+fixtureKey,
		"LIC-"+fixtureKey,
		now.AddDate(5, 0, 0),
		now,
		now,
	)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf(
			"create test driver: %w",
			err,
		)
	}

	// Active assignment.
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO driver_assignments (
				company_id,
				branch_id,
				fleet_id,
				driver_id,
				vehicle_id,
				assigned_at,
				notes
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				'Disposable CONNECT integration-test assignment'
			)
			RETURNING id
		`,
		fixture.CompanyID,
		fixture.BranchID,
		fixture.FleetID,
		fixture.UserID,
		fixture.VehicleID,
		now,
	).Scan(&fixture.AssignmentID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf(
			"create test driver assignment: %w",
			err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		rollback()
		return nil, nil, fmt.Errorf(
			"commit driver fixture: %w",
			err,
		)
	}

	cleanup := func(cleanupCtx context.Context) error {
		cleanupTx, err := db.Begin(cleanupCtx)
		if err != nil {
			return fmt.Errorf(
				"begin driver fixture cleanup: %w",
				err,
			)
		}

		defer func() {
			_ = cleanupTx.Rollback(context.Background())
		}()

		queries := []struct {
			name string
			sql  string
			id   string
		}{
			{
				name: "driver assignment",
				sql:  `DELETE FROM driver_assignments WHERE id = $1`,
				id:   fixture.AssignmentID,
			},
			{
				name: "driver",
				sql:  `DELETE FROM drivers WHERE id = $1`,
				id:   fixture.DriverID,
			},
			{
				name: "vehicle",
				sql:  `DELETE FROM vehicles WHERE id = $1`,
				id:   fixture.VehicleID,
			},
			{
				name: "user",
				sql:  `DELETE FROM users WHERE id = $1`,
				id:   fixture.UserID,
			},
			{
				name: "fleet",
				sql:  `DELETE FROM fleets WHERE id = $1`,
				id:   fixture.FleetID,
			},
			{
				name: "branch",
				sql:  `DELETE FROM branches WHERE id = $1`,
				id:   fixture.BranchID,
			},
			{
				name: "company",
				sql:  `DELETE FROM companies WHERE id = $1`,
				id:   fixture.CompanyID,
			},
		}

		for _, query := range queries {
			if _, err := cleanupTx.Exec(
				cleanupCtx,
				query.sql,
				query.id,
			); err != nil {
				return fmt.Errorf(
					"delete test %s: %w",
					query.name,
					err,
				)
			}
		}

		if err := cleanupTx.Commit(cleanupCtx); err != nil {
			return fmt.Errorf(
				"commit driver fixture cleanup: %w",
				err,
			)
		}

		return nil
	}

	return fixture, cleanup, nil
}
