package flex_offer

import (
	"context"
	"errors"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/testutil"
	"github.com/google/uuid"
)

func TestConfirmAuthorizedRideRejectsInvalidPricingProvenance(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *testing.T, *testutil.FlexAcceptFixture)
	}{
		{
			name: "pricing version mismatch",
			mutate: func(ctx context.Context, t *testing.T, f *testutil.FlexAcceptFixture) {
				t.Helper()

				_, err := f.DB.Exec(ctx, `
					UPDATE flex_auctions
					SET pricing_version = $2
					WHERE id = $1
				`, f.AuctionID, "invalid-version-"+uuid.NewString()[:8])
				if err != nil {
					t.Fatalf("change frozen pricing version: %v", err)
				}
			},
		},
		{
			name: "pricing company mismatch",
			mutate: func(ctx context.Context, t *testing.T, f *testutil.FlexAcceptFixture) {
				t.Helper()

				// Use an existing company-owned profile only when its
				// company differs from the selected driver's company.
				var foreignProfileID string
				var foreignVersion string

				err := f.DB.QueryRow(ctx, `
					SELECT fp.id, fp.version
					FROM fare_pricing_profiles fp
					JOIN flex_auctions fa ON fa.id = $1
					JOIN driver_assignments da ON da.id = (
						SELECT assignment_id
						FROM flex_offers
						WHERE auction_id = fa.id
						  AND status = 'SELECTED'
						LIMIT 1
					)
					WHERE fp.company_id <> da.company_id
					  AND fp.currency = 'EUR'
					ORDER BY fp.created_at DESC
					LIMIT 1
				`, f.AuctionID).Scan(
					&foreignProfileID,
					&foreignVersion,
				)
				if err != nil {
					t.Fatalf("resolve foreign-company pricing profile: %v", err)
				}

				_, err = f.DB.Exec(ctx, `
					UPDATE flex_auctions
					SET pricing_profile_id = $2,
					    pricing_version = $3
					WHERE id = $1
				`, f.AuctionID, foreignProfileID, foreignVersion)
				if err != nil {
					t.Fatalf("change frozen pricing company: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := testutil.SetupFlexAcceptFixture(t)
			ctx := context.Background()

			service := NewService(Dependencies{
				DB:     f.DB,
				Config: f.Config,
			})

			// Cleanup must execute before shared fixture cleanup.
			t.Cleanup(func() {
				cleanupCtx := context.Background()

				if _, err := f.DB.Exec(cleanupCtx, `
					DELETE FROM trips
					WHERE ride_request_id = $1
				`, f.RideID); err != nil {
					t.Errorf("cleanup rejected FLEX trip: %v", err)
				}

				if _, err := f.DB.Exec(cleanupCtx, `
					DELETE FROM flex_authorizations
					WHERE auction_id = $1
				`, f.AuctionID); err != nil {
					t.Errorf("cleanup FLEX authorization: %v", err)
				}

				if _, err := f.DB.Exec(cleanupCtx, `
					UPDATE flex_auctions
					SET status = 'OPEN',
					    selected_offer_id = NULL,
					    resolved_at = NULL,
					    updated_at = clock_timestamp()
					WHERE id = $1
					  AND status IN ('PENDING_AUTHORIZATION', 'CONFIRMED')
				`, f.AuctionID); err != nil {
					t.Errorf("cleanup selected FLEX auction: %v", err)
				}
			})

			offer, err := service.SubmitAcceptFare(
				ctx, f.AuctionID, f.DriverID,
			)
			if err != nil {
				t.Fatalf("submit FLEX offer: %v", err)
			}

			var passengerID string
			if err := f.DB.QueryRow(ctx, `
				SELECT customer_id
				FROM ride_requests
				WHERE id = $1
			`, f.RideID).Scan(&passengerID); err != nil {
				t.Fatalf("resolve FLEX passenger: %v", err)
			}

			if _, err := service.SelectPassengerOffer(
				ctx, f.RideID, f.AuctionID, offer.ID, passengerID,
			); err != nil {
				t.Fatalf("select FLEX offer: %v", err)
			}

			authorization, err := postgresrepo.CreateFlexAuthorization(
				ctx, f.DB, f.RideID, f.AuctionID, passengerID,
			)
			if err != nil {
				t.Fatalf("create FLEX authorization: %v", err)
			}

			_, claimed, err := postgresrepo.ClaimFlexAuthorizationProcessing(
				ctx, f.DB, authorization.ID,
			)
			if err != nil || !claimed {
				t.Fatalf(
					"claim FLEX authorization: claimed=%v err=%v",
					claimed, err,
				)
			}

			providerID := "pi_flex_pricing_" + uuid.NewString()

			authorized, err := postgresrepo.ReconcileFlexAuthorization(
				ctx,
				f.DB,
				authorization.ID,
				providerID,
				postgresrepo.FlexResultAuthorized,
			)
			if err != nil {
				t.Fatalf("authorize FLEX payment: %v", err)
			}
			if authorized == nil || authorized.Status != "AUTHORIZED" {
				t.Fatalf("unexpected authorization: %+v", authorized)
			}

			tt.mutate(ctx, t, f)

			trip, err := service.ConfirmAuthorizedRide(
				ctx, f.RideID, f.AuctionID, passengerID,
			)
			if !errors.Is(err, ErrFlexConfirmationNotEligible) {
				t.Fatalf(
					"expected pricing provenance rejection: trip=%+v err=%v",
					trip, err,
				)
			}
			if trip != nil {
				t.Fatalf("invalid pricing provenance created trip: %+v", trip)
			}

			var tripCount int
			if err := f.DB.QueryRow(ctx, `
				SELECT COUNT(*)
				FROM trips
				WHERE ride_request_id = $1
			`, f.RideID).Scan(&tripCount); err != nil {
				t.Fatalf("count FLEX trips: %v", err)
			}
			if tripCount != 0 {
				t.Fatalf("invalid pricing provenance created %d trips", tripCount)
			}

			var rideStatus string
			var offerStatus string
			var auctionStatus string
			var presenceStatus string
			var authorizationStatus string

			checks := []struct {
				query string
				id    string
				dest  *string
			}{
				{
					"SELECT status FROM ride_requests WHERE id = $1",
					f.RideID,
					&rideStatus,
				},
				{
					"SELECT status FROM flex_offers WHERE id = $1",
					offer.ID,
					&offerStatus,
				},
				{
					"SELECT status FROM flex_auctions WHERE id = $1",
					f.AuctionID,
					&auctionStatus,
				},
				{
					"SELECT availability_status FROM driver_presence WHERE driver_id = $1",
					f.DriverID,
					&presenceStatus,
				},
				{
					"SELECT status FROM flex_authorizations WHERE id = $1",
					authorization.ID,
					&authorizationStatus,
				},
			}

			for _, check := range checks {
				if err := f.DB.QueryRow(
					ctx, check.query, check.id,
				).Scan(check.dest); err != nil {
					t.Fatalf("verify unchanged FLEX state: %v", err)
				}
			}

			if rideStatus != "PENDING" ||
				offerStatus != "SELECTED" ||
				auctionStatus != "PENDING_AUTHORIZATION" ||
				presenceStatus != "AVAILABLE" ||
				authorizationStatus != "AUTHORIZED" {
				t.Fatalf(
					"pricing rejection changed state: ride=%s offer=%s auction=%s presence=%s authorization=%s",
					rideStatus,
					offerStatus,
					auctionStatus,
					presenceStatus,
					authorizationStatus,
				)
			}
		})
	}
}
