package trip

import (
	"context"
	"os"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	stripego "github.com/stripe/stripe-go/v86"
)

func TestFlexCaptureOrchestratorUnresolvedProvider(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	tests := []struct {
		name           string
		claim          bool
		providerStatus stripego.PaymentIntentStatus
		expected       flexsettlement.RecoveryStatus
		localStatus    string
	}{
		{
			name:           "pending authorized funds",
			providerStatus: stripego.PaymentIntentStatusRequiresCapture,
			expected:       flexsettlement.RecoveryPendingProvider,
			localStatus:    "PENDING",
		},
		{
			name:           "processing authorized funds",
			claim:          true,
			providerStatus: stripego.PaymentIntentStatusRequiresCapture,
			expected:       flexsettlement.RecoveryReadyForCapture,
			localStatus:    "PROCESSING",
		},
		{
			name:           "provider still processing",
			claim:          true,
			providerStatus: stripego.PaymentIntentStatusProcessing,
			expected:       flexsettlement.RecoveryPendingProvider,
			localStatus:    "PROCESSING",
		},
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
					"refusing settlement test on %q",
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
					ProviderStatus:    tt.providerStatus,
					Captured:          false,
				},
			}

			orchestrator := flexsettlement.NewOrchestrator(
				db,
				flexsettlement.NewRecoveryService(db, verifier),
			)

			result, err := orchestrator.Reconcile(ctx, settlement.ID)
			if err != nil {
				t.Fatalf("reconcile unresolved provider: %v", err)
			}
			if result == nil || result.Status != tt.expected {
				t.Fatalf(
					"expected recovery %s, got %+v",
					tt.expected,
					result,
				)
			}
			if verifier.calls != 1 {
				t.Fatalf(
					"expected one verification, got %d",
					verifier.calls,
				)
			}

			var status string
			var paymentCount int
			var transactionCount int
			var earningCount int

			err = db.QueryRow(ctx, `
				SELECT
					fs.status,
					(
						SELECT COUNT(*)
						FROM payments
						WHERE trip_id = fs.trip_id
					),
					(
						SELECT COUNT(*)
						FROM payment_transactions pt
						JOIN payments p ON p.id = pt.payment_id
						WHERE p.trip_id = fs.trip_id
						  AND pt.transaction_type = 'CAPTURE'
					),
					(
						SELECT COUNT(*)
						FROM driver_earnings
						WHERE trip_id = fs.trip_id
					)
				FROM flex_settlements fs
				WHERE fs.id = $1
			`, settlement.ID).Scan(
				&status,
				&paymentCount,
				&transactionCount,
				&earningCount,
			)
			if err != nil {
				t.Fatalf("inspect unresolved settlement: %v", err)
			}

			if status != tt.localStatus ||
				paymentCount != 0 ||
				transactionCount != 0 ||
				earningCount != 0 {
				t.Fatalf(
					"unexpected financial mutation: "+
						"status=%s payments=%d captures=%d earnings=%d",
					status,
					paymentCount,
					transactionCount,
					earningCount,
				)
			}
		})
	}
}
