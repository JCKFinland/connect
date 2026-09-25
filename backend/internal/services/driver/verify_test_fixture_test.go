package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type verificationFixture struct {
	CompanyID  string
	BranchID   string
	UserID     string
	DriverID   string
	VerifierID string
}

func createVerificationFixture(
	ctx context.Context,
	db *pgxpool.Pool,
) (*verificationFixture, func(context.Context) error, error) {
	fixture := &verificationFixture{}

	key := uuid.NewString()
	shortKey := key[:8]
	now := time.Now().UTC()

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"begin verification fixture transaction: %w",
			err,
		)
	}

	rollback := func() {
		_ = tx.Rollback(context.Background())
	}

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
				$1, $2, $3, $4, $5,
				'FI',
				'Europe/Helsinki',
				'Espoo',
				TRUE
			)
			RETURNING id
		`,
		"CONNECT Verification Company "+shortKey,
		"CONNECT Verification Company "+shortKey,
		"VERIFY-"+key,
		"company-"+key+"@example.test",
		"+358000000011",
	).Scan(&fixture.CompanyID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf("create verification company: %w", err)
	}

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
				$1, $2, $3, $4, $5,
				'Espoo',
				60.2055,
				24.6559,
				TRUE
			)
			RETURNING id
		`,
		fixture.CompanyID,
		"VERIFY-"+shortKey,
		"CONNECT Verification Branch "+shortKey,
		"branch-"+key+"@example.test",
		"+358000000012",
	).Scan(&fixture.BranchID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf("create verification branch: %w", err)
	}

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
			VALUES ($1, $2, 'Pending', 'Driver', $3)
			RETURNING id
		`,
		"pending-driver-"+key+"@example.test",
		"test-fixture-password-hash",
		"+358000000013",
	).Scan(&fixture.UserID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf("create pending driver user: %w", err)
	}

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
			VALUES ($1, $2, 'Verification', 'Admin', $3)
			RETURNING id
		`,
		"verifier-"+key+"@example.test",
		"test-fixture-password-hash",
		"+358000000014",
	).Scan(&fixture.VerifierID)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf("create verifier user: %w", err)
	}

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
				status,
				is_verified,
				is_active,
				created_at,
				updated_at
			)
			VALUES (
				$1, $2, $3, $4, $5,
				'Pending',
				'Driver',
				$6, $7, $8, $9, $10,
				'PENDING_VERIFICATION',
				FALSE,
				TRUE,
				$11,
				$11
			)
		`,
		fixture.DriverID,
		fixture.UserID,
		fixture.CompanyID,
		fixture.BranchID,
		"DRV-"+key,
		"+358000000013",
		"pending-driver-"+key+"@example.test",
		"TAXI-"+key,
		"LIC-"+key,
		now.AddDate(5, 0, 0),
		now,
	)
	if err != nil {
		rollback()
		return nil, nil, fmt.Errorf("create pending driver: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf(
			"commit verification fixture: %w",
			err,
		)
	}

	cleanup := func(cleanupCtx context.Context) error {
		tx, err := db.Begin(cleanupCtx)
		if err != nil {
			return fmt.Errorf("begin verification fixture cleanup: %w", err)
		}

		defer func() {
			_ = tx.Rollback(context.Background())
		}()

		if _, err := tx.Exec(
			cleanupCtx,
			`DELETE FROM drivers WHERE id = $1`,
			fixture.DriverID,
		); err != nil {
			return fmt.Errorf("delete verification driver: %w", err)
		}

		if _, err := tx.Exec(
			cleanupCtx,
			`DELETE FROM users WHERE id IN ($1, $2)`,
			fixture.UserID,
			fixture.VerifierID,
		); err != nil {
			return fmt.Errorf("delete verification users: %w", err)
		}

		if _, err := tx.Exec(
			cleanupCtx,
			`DELETE FROM branches WHERE id = $1`,
			fixture.BranchID,
		); err != nil {
			return fmt.Errorf("delete verification branch: %w", err)
		}

		if _, err := tx.Exec(
			cleanupCtx,
			`DELETE FROM companies WHERE id = $1`,
			fixture.CompanyID,
		); err != nil {
			return fmt.Errorf("delete verification company: %w", err)
		}

		if err := tx.Commit(cleanupCtx); err != nil {
			return fmt.Errorf("commit verification fixture cleanup: %w", err)
		}

		return nil
	}

	return fixture, cleanup, nil
}
