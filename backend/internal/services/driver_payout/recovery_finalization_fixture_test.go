package driver_payout

import (
	"context"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recoveryFinancialFixture struct {
	payoutID  string
	earningID string
	paymentID string
	tripID    string
}

func newRecoveryFinancialFixture(
	t *testing.T,
	db *pgxpool.Pool,
	payoutID string,
	driverID string,
	companyID string,
) *recoveryFinancialFixture {
	t.Helper()

	ctx := context.Background()

	var actualDB string
	var migration int

	err := db.QueryRow(ctx, `
        SELECT current_database(),
               COALESCE(
                   (SELECT MAX(version_id)
                    FROM goose_db_version
                    WHERE is_applied=true), 0)
    `).Scan(&actualDB, &migration)

	if err != nil ||
		actualDB != "connect_payout_destination_test" ||
		migration != 75 {
		t.Fatalf(
			"unsafe financial fixture database: %s migration=%d err=%v",
			actualDB, migration, err,
		)
	}

	customerID, cleanupCustomer, err :=
		testutil.CreateCustomerFixture(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(), 20*time.Second,
		)
		defer cancel()

		if err := cleanupCustomer(cleanupCtx); err != nil {
			t.Errorf("cleanup customer: %v", err)
		}
	})

	// Resolve organizational records belonging to the payout company.
	// A dedicated fixture must create these before this helper runs.
	var vehicleID, branchID, fleetID string

	err = db.QueryRow(ctx, `
        SELECT v.id::text,
               v.branch_id::text,
               v.fleet_id::text
        FROM vehicles v
        WHERE v.company_id=$1::uuid
          AND v.branch_id IS NOT NULL
          AND v.fleet_id IS NOT NULL
        ORDER BY v.id
        LIMIT 1
    `, companyID).Scan(&vehicleID, &branchID, &fleetID)

	if err != nil {
		t.Fatalf(
			"missing payout-company vehicle fixture: %v",
			err,
		)
	}

	rideID := uuid.NewString()
	tripID := uuid.NewString()
	now := time.Now().UTC()

	_, err = db.Exec(ctx, `
        INSERT INTO ride_requests (
            id, customer_id,
            pickup_address, pickup_latitude, pickup_longitude,
            destination_address,
            destination_latitude, destination_longitude,
            requested_vehicle_type, passenger_count,
            status, matching_mode,
            requested_at, expires_at
        )
        VALUES (
            $1, $2,
            'Recovery Fixture Pickup', 60.2055, 24.6559,
            'Recovery Fixture Destination', 60.1719, 24.9414,
            'STANDARD', 1,
            'ACCEPTED', 'FLEX',
            $3, $4
        )
    `, rideID, customerID, now, now.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(),
			`DELETE FROM ride_requests WHERE id=$1`, rideID)
		if err != nil {
			t.Errorf("cleanup ride request: %v", err)
		}
	})

	_, err = db.Exec(ctx, `
        INSERT INTO trips (
            id, ride_request_id,
            customer_id, driver_id,
            vehicle_id, company_id,
            branch_id, fleet_id,
            status, is_active,
            assigned_at, driver_arrived_at,
            started_at, completed_at
        )
        VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8,
            'COMPLETED', FALSE,
            $9, $10, $10, $11
        )
    `,
		tripID, rideID, customerID, driverID,
		vehicleID, companyID,
		branchID, fleetID,
		now.Add(-20*time.Minute),
		now.Add(-10*time.Minute), now,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(),
			`DELETE FROM trips WHERE id=$1`, tripID)
		if err != nil {
			t.Errorf("cleanup trip: %v", err)
		}
	})

	_, err = db.Exec(ctx, `
        INSERT INTO trip_fares (
            trip_id, base_fare,
            total_amount, currency,
            surge_multiplier, pricing_version
        )
        VALUES ($1, 100, 100, 'EUR', 1, 'recovery-fixture-v1')
    `, tripID)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(),
			`DELETE FROM trip_fares WHERE trip_id=$1::uuid`, tripID)
		if err != nil {
			t.Errorf("cleanup trip fare: %v", err)
		}
	})

	paymentRepo := postgresrepo.NewPaymentRepository(db)

	payment, err := paymentRepo.CreateFromCompletedTrip(
		ctx, tripID, "CARD",
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := paymentRepo.UpdateStatus(
		ctx, payment.ID, "PAID",
	); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(),
			`DELETE FROM payments WHERE id=$1`, payment.ID)
		if err != nil {
			t.Errorf("cleanup payment: %v", err)
		}
	})

	earning, err :=
		postgresrepo.NewDriverEarningRepository(db).
			CreateFromPaidPayment(ctx, payment.ID)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(),
			`DELETE FROM driver_earnings WHERE id=$1`, earning.ID)
		if err != nil {
			t.Errorf("cleanup earning: %v", err)
		}
	})

	if earning.GrossAmount != "100.00" ||
		earning.CommissionAmount != "7.00" ||
		earning.NetAmount != "93.00" {
		t.Fatalf(
			"unexpected FLEX earning: gross=%s commission=%s net=%s",
			earning.GrossAmount,
			earning.CommissionAmount,
			earning.NetAmount,
		)
	}

	_, err = db.Exec(ctx, `
        INSERT INTO driver_payout_items (
            payout_id, earning_id, amount
        )
        VALUES ($1::uuid, $2::uuid, 93)
    `, payoutID, earning.ID)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_, err := db.Exec(context.Background(), `
            DELETE FROM driver_payout_items
            WHERE payout_id=$1::uuid AND earning_id=$2::uuid
        `, payoutID, earning.ID)
		if err != nil {
			t.Errorf("cleanup payout item: %v", err)
		}
	})

	tag, err := db.Exec(ctx, `
        UPDATE driver_earnings
        SET settlement_status='PROCESSING',
            updated_at=NOW()
        WHERE id=$1::uuid
          AND settlement_status='PENDING'
    `, earning.ID)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf(
			"commit earning to payout: rows=%d err=%v",
			tag.RowsAffected(), err,
		)
	}

	return &recoveryFinancialFixture{
		payoutID:  payoutID,
		earningID: earning.ID,
		paymentID: payment.ID,
		tripID:    tripID,
	}
}
