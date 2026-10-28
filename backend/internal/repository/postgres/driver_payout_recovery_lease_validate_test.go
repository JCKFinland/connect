package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateDriverPayoutRecoveryLeaseInvalidInput(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" {
		t.Skip("requires isolated payout destination database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must be disabled")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	config, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_NAME"),
	))
	if err != nil {
		t.Fatal(err)
	}

	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var database string
	var version int64

	err = db.QueryRow(ctx, `
        SELECT current_database(),
               COALESCE(
                   (SELECT MAX(version_id)
                    FROM goose_db_version
                    WHERE is_applied=true),
                   0
               )
    `).Scan(&database, &version)
	if err != nil {
		t.Fatal(err)
	}

	if database != "connect_payout_destination_test" ||
		version != 75 {
		t.Fatalf(
			"unsafe database %s migration %d",
			database, version,
		)
	}

	payoutID := uuid.NewString()
	token := uuid.NewString()

	tests := []struct {
		name      string
		payoutID  string
		token     string
		remaining time.Duration
	}{
		{"invalid payout", "invalid", token, time.Second},
		{"invalid token", payoutID, "invalid", time.Second},
		{"zero duration", payoutID, token, 0},
		{"negative duration", payoutID, token, -time.Second},
		{"excessive duration", payoutID, token, 6 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := ValidateDriverPayoutRecoveryLease(
				ctx,
				db,
				tt.payoutID,
				tt.token,
				tt.remaining,
			)

			if valid || !errors.Is(
				err,
				ErrDriverPayoutRecoveryLeaseInvalid,
			) {
				t.Fatalf(
					"valid=%v err=%v",
					valid, err,
				)
			}
		})
	}

	t.Run("nil database pool", func(t *testing.T) {
		valid, err := ValidateDriverPayoutRecoveryLease(
			ctx,
			nil,
			payoutID,
			token,
			time.Second,
		)

		if valid || !errors.Is(
			err,
			ErrDriverPayoutRecoveryLeaseInvalid,
		) {
			t.Fatalf(
				"valid=%v err=%v",
				valid, err,
			)
		}
	})
}

func TestValidateDriverPayoutRecoveryLeaseDatabase(t *testing.T) {
	if os.Getenv("DB_NAME") != "connect_payout_destination_test" {
		t.Skip("requires isolated payout destination database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("settlement worker must be disabled")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		60*time.Second,
	)
	defer cancel()

	config, err := pgxpool.ParseConfig(fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_NAME"),
	))
	if err != nil {
		t.Fatal(err)
	}

	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	// Close the pool after fixture cleanup completes.
	// t.Cleanup executes callbacks in reverse order.
	t.Cleanup(db.Close)

	var database string
	var version int64

	err = db.QueryRow(ctx, `
        SELECT current_database(),
               COALESCE(MAX(version_id), 0)
        FROM goose_db_version
        WHERE is_applied = true
        GROUP BY current_database()
    `).Scan(&database, &version)
	if err != nil {
		t.Fatal(err)
	}

	if database != "connect_payout_destination_test" ||
		version != 75 {
		t.Fatalf(
			"unsafe database %s version %d",
			database, version,
		)
	}

	driverID := uuid.NewString()
	companyID := uuid.NewString()
	payoutID := uuid.NewString()
	token := uuid.NewString()
	replacementToken := uuid.NewString()
	suffix := uuid.NewString()

	// Cleanup is registered before fixture creation.
	// Only records identified by these generated UUIDs are removed.
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(
			context.Background(),
			20*time.Second,
		)
		defer stop()

		statements := []struct {
			sql string
			id  string
		}{
			{
				"DELETE FROM driver_payout_recovery_leases WHERE payout_id=$1",
				payoutID,
			},
			{
				"DELETE FROM driver_payouts WHERE id=$1",
				payoutID,
			},
			{
				"DELETE FROM companies WHERE id=$1",
				companyID,
			},
			{
				"DELETE FROM users WHERE id=$1",
				driverID,
			},
		}

		for _, statement := range statements {
			if _, err := db.Exec(
				cleanupCtx,
				statement.sql,
				statement.id,
			); err != nil {
				t.Errorf("fixture cleanup: %v", err)
			}
		}
	})

	_, err = db.Exec(ctx, `
        INSERT INTO users (
            id, email, password_hash,
            first_name, last_name
        )
        VALUES ($1, $2, 'integration-test-only',
                'Lease', 'Fixture')
    `,
		driverID,
		"lease-"+suffix+"@example.invalid",
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO companies (
            id, name, legal_name,
            country_code, timezone
        )
        VALUES ($1, $2, $2, 'FI', 'Europe/Helsinki')
    `,
		companyID,
		"Lease Fixture "+suffix,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO driver_payouts (
            id, driver_id, company_id,
            payout_reference,
            period_start, period_end,
            gross_amount, deductions, adjustments,
            net_amount, currency,
            payout_method, payout_status
        )
        VALUES (
            $1, $2, $3, $4,
            NOW()-INTERVAL '2 days',
            NOW()-INTERVAL '1 day',
            100, 7, 0, 93,
            'EUR', 'BANK_TRANSFER', 'PENDING'
        )
    `,
		payoutID,
		driverID,
		companyID,
		"lease-test-"+suffix,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO driver_payout_recovery_leases (
            payout_id,
            lease_token,
            lease_owner,
            lease_expires_at,
            next_lookup_at,
            last_lookup_at,
            lookup_count
        )
        VALUES (
            $1::uuid,
            $2::uuid,
            'lease-validation-test',
            clock_timestamp() + interval '90 seconds',
            clock_timestamp() + interval '5 minutes',
            clock_timestamp(),
            1
        )
    `, payoutID, token)
	if err != nil {
		t.Fatal(err)
	}

	check := func(
		name string,
		checkToken string,
		remaining time.Duration,
		want bool,
	) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			valid, err := ValidateDriverPayoutRecoveryLease(
				ctx,
				db,
				payoutID,
				checkToken,
				remaining,
			)
			if err != nil {
				t.Fatal(err)
			}
			if valid != want {
				t.Fatalf(
					"valid=%v want=%v",
					valid, want,
				)
			}
		})
	}

	check("valid token", token, 35*time.Second, true)
	check("incorrect token", replacementToken, 35*time.Second, false)
	check("insufficient lifetime", token, 2*time.Minute, false)

	_, err = db.Exec(ctx, `
        UPDATE driver_payout_recovery_leases
        SET lease_expires_at =
            clock_timestamp() - interval '1 second'
        WHERE payout_id=$1::uuid
    `, payoutID)
	if err != nil {
		t.Fatal(err)
	}

	check("expired lease", token, 35*time.Second, false)

	_, err = db.Exec(ctx, `
        UPDATE driver_payout_recovery_leases
        SET lease_token=$2::uuid,
            lease_expires_at =
                clock_timestamp() + interval '90 seconds'
        WHERE payout_id=$1::uuid
    `, payoutID, replacementToken)
	if err != nil {
		t.Fatal(err)
	}

	check("replaced old token", token, 35*time.Second, false)
	check("replacement token", replacementToken, 35*time.Second, true)

	t.Log("PASS: production lease validation rejects stale ownership")
}
