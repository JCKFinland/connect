package trip

import (
	"context"
	"os"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	"github.com/jackc/pgx/v5/pgxpool"
	stripego "github.com/stripe/stripe-go/v86"
)

func TestFlexCaptureOrchestratorCapturedReconciliation(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	tests := []struct {
		name  string
		claim bool
	}{
		{name: "pending settlement"},
		{name: "processing settlement", claim: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle := completeAuthorizedFlexLifecycle(t)

			ctx := context.Background()
			db := lifecycle.Fixture.DB

			var databaseName string
			if err := db.QueryRow(
				ctx,
				"SELECT current_database()",
			).Scan(&databaseName); err != nil {
				t.Fatal(err)
			}

			if databaseName != "connect_settlement_test" {
				t.Fatalf(
					"refusing settlement integration test on %q",
					databaseName,
				)
			}

			settlement, err := postgres.CreateFlexSettlementFromCompletedTrip(
				ctx,
				db,
				lifecycle.TripID,
			)
			if err != nil {
				t.Fatalf("create settlement: %v", err)
			}

			if tt.claim {
				_, claimed, err := postgres.ClaimFlexSettlementProcessing(
					ctx,
					db,
					settlement.ID,
				)
				if err != nil {
					t.Fatalf("claim settlement: %v", err)
				}
				if !claimed {
					t.Fatal("expected processing claim")
				}
			}

			verifier := &flexRecoveryFakeVerifier{
				result: &stripepayment.FlexCaptureVerificationResult{
					ProviderCaptureID: lifecycle.ProviderID,
					ProviderStatus:    stripego.PaymentIntentStatusSucceeded,
					Captured:          true,
				},
			}

			recovery := flexsettlement.NewRecoveryService(db, verifier)
			orchestrator := flexsettlement.NewOrchestrator(db, recovery)

			result, err := orchestrator.Reconcile(ctx, settlement.ID)
			if err != nil {
				t.Fatalf("reconcile captured settlement: %v", err)
			}

			if result == nil ||
				result.Status != flexsettlement.RecoveryCaptured {
				t.Fatalf("unexpected recovery result: %+v", result)
			}

			assertCapturedFlexOrchestratorState(
				t,
				ctx,
				db,
				settlement.ID,
				lifecycle.TripID,
				lifecycle.ProviderID,
				lifecycle.AmountCents,
			)

			replay, err := orchestrator.Reconcile(ctx, settlement.ID)
			if err != nil {
				t.Fatalf("reconcile finalized settlement: %v", err)
			}

			if replay == nil ||
				replay.Status != flexsettlement.RecoveryAlreadyFinalized {
				t.Fatalf("unexpected replay result: %+v", replay)
			}

			if verifier.calls != 1 {
				t.Fatalf(
					"expected one Stripe verification, got %d",
					verifier.calls,
				)
			}

			assertCapturedFlexOrchestratorState(
				t,
				ctx,
				db,
				settlement.ID,
				lifecycle.TripID,
				lifecycle.ProviderID,
				lifecycle.AmountCents,
			)
		})
	}
}

func assertCapturedFlexOrchestratorState(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
	settlementID string,
	tripID string,
	providerID string,
	amountCents int64,
) {
	t.Helper()

	var settlementStatus string
	var paymentStatus string
	var captureStatus string
	var persistedProviderID string
	var grossCents int64
	var commissionCents int64
	var netCents int64

	err := db.QueryRow(ctx, `
		SELECT
			fs.status,
			p.status,
			pt.status,
			fs.provider_capture_id,
			(de.gross_amount * 100)::bigint,
			(de.commission_amount * 100)::bigint,
			(de.net_amount * 100)::bigint
		FROM flex_settlements fs
		JOIN payments p ON p.id = fs.payment_id
		JOIN payment_transactions pt
			ON pt.id = fs.capture_transaction_id
		JOIN driver_earnings de ON de.trip_id = fs.trip_id
		WHERE fs.id = $1
	`, settlementID).Scan(
		&settlementStatus,
		&paymentStatus,
		&captureStatus,
		&persistedProviderID,
		&grossCents,
		&commissionCents,
		&netCents,
	)
	if err != nil {
		t.Fatalf("read reconciled financial state: %v", err)
	}

	expectedCommission := (amountCents*7 + 50) / 100

	if settlementStatus != "CAPTURED" ||
		paymentStatus != "PAID" ||
		captureStatus != "SUCCESS" ||
		persistedProviderID != providerID ||
		grossCents != amountCents ||
		commissionCents != expectedCommission ||
		netCents != amountCents-expectedCommission {
		t.Fatalf(
			"invalid financial state: settlement=%s payment=%s "+
				"capture=%s provider=%s gross=%d commission=%d net=%d",
			settlementStatus,
			paymentStatus,
			captureStatus,
			persistedProviderID,
			grossCents,
			commissionCents,
			netCents,
		)
	}

	for _, query := range []string{
		`SELECT COUNT(*) FROM payments WHERE trip_id = $1`,
		`SELECT COUNT(*) FROM driver_earnings WHERE trip_id = $1`,
		`SELECT COUNT(*)
		 FROM payment_transactions pt
		 JOIN payments p ON p.id = pt.payment_id
		 WHERE p.trip_id = $1 AND pt.transaction_type = 'CAPTURE'`,
	} {
		var count int
		if err := db.QueryRow(ctx, query, tripID).Scan(&count); err != nil {
			t.Fatalf("count financial records: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected one financial record, got %d", count)
		}
	}
}
