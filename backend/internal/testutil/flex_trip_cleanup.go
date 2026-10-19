package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterFlexTripCleanup registers cleanup for trip and authorization
// records created after SetupFlexAcceptFixture.
//
// Call this immediately after SetupFlexAcceptFixture. Go testing
// executes cleanup callbacks in reverse registration order, so these
// records are removed before the underlying fixture is destroyed.
//
// Cleanup is restricted to the fixture's ride request and auction.
func RegisterFlexTripCleanup(
	t *testing.T,
	db *pgxpool.Pool,
	rideID string,
	auctionID string,
) {
	t.Helper()

	if db == nil || rideID == "" || auctionID == "" {
		t.Fatal("invalid FLEX trip cleanup dependencies")
	}

	t.Cleanup(func() {
		ctx := context.Background()

		// Settlement integration records must be removed before
		// their payment, authorization, and trip parents.
		// Never run this financial cleanup against the main CONNECT DB.
		if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") == "1" {
			var databaseName string
			if err := db.QueryRow(
				ctx,
				"SELECT current_database()",
			).Scan(&databaseName); err != nil {
				t.Errorf("verify settlement cleanup database: %v", err)
				return
			}
			if databaseName != "connect_settlement_test" {
				t.Errorf(
					"refusing financial cleanup on database %q",
					databaseName,
				)
				return
			}

			for _, query := range []string{
				`DELETE FROM driver_earnings de
				 WHERE de.trip_id IN (
				     SELECT id FROM trips
				     WHERE ride_request_id = $1
				 )
				 AND NOT EXISTS (
				     SELECT 1 FROM driver_payout_items dpi
				     WHERE dpi.earning_id = de.id
				 )`,
				`DELETE FROM flex_settlements
				 WHERE trip_id IN (
				   SELECT id FROM trips WHERE ride_request_id = $1
				 )`,
				`DELETE FROM payment_transactions
				 WHERE payment_id IN (
				   SELECT p.id FROM payments p
				   JOIN trips t ON t.id = p.trip_id
				   WHERE t.ride_request_id = $1
				 )`,
				`DELETE FROM payments
				 WHERE trip_id IN (
				   SELECT id FROM trips WHERE ride_request_id = $1
				 )`,
			} {
				if _, err := db.Exec(ctx, query, rideID); err != nil {
					t.Errorf("cleanup FLEX financial records: %v", err)
					return
				}
			}
		}

		// Child records with restrictive foreign keys must be
		// handled before deleting their parent trip.
		//
		// Settlement test earnings are removed above before their
		// financial parents and trip records.
		if _, err := db.Exec(ctx, `
			DELETE FROM trips
			WHERE ride_request_id = $1
		`, rideID); err != nil {
			t.Errorf("cleanup FLEX trips: %v", err)
		}

		// Authorization references the selected offer through a
		// restrictive composite foreign key.
		if _, err := db.Exec(ctx, `
			DELETE FROM flex_authorizations
			WHERE auction_id = $1
		`, auctionID); err != nil {
			t.Errorf("cleanup FLEX authorizations: %v", err)
		}

		// Remove the auction's reference to its selected offer.
		if _, err := db.Exec(ctx, `
			UPDATE flex_auctions
			SET selected_offer_id = NULL,
			    resolved_at = NULL,
			    status = 'OPEN',
			    updated_at = clock_timestamp()
			WHERE id = $1
			  AND status IN (
			      'PENDING_AUTHORIZATION',
			      'CONFIRMED',
                           'CANCELLED'
			  )
		`, auctionID); err != nil {
			t.Errorf("reset FLEX auction for cleanup: %v", err)
		}
	})
}
