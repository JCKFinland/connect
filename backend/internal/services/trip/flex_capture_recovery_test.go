package trip

import (
	"context"
	"errors"
	"os"
	"testing"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
	stripego "github.com/stripe/stripe-go/v86"
)

type flexRecoveryFakeVerifier struct {
	result  *stripepayment.FlexCaptureVerificationResult
	err     error
	calls   int
	request stripepayment.FlexCaptureVerificationRequest
}

func (f *flexRecoveryFakeVerifier) VerifyFlexCapture(
	_ context.Context,
	req stripepayment.FlexCaptureVerificationRequest,
) (*stripepayment.FlexCaptureVerificationResult, error) {
	f.calls++
	f.request = req
	return f.result, f.err
}

func TestFlexCaptureRecoveryIntegration(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}

	tests := []struct {
		name           string
		claim          bool
		providerStatus stripego.PaymentIntentStatus
		captured       bool
		providerError  error
		wantStatus     flexsettlement.RecoveryStatus
		wantError      bool
	}{
		{
			name:           "pending authorized funds",
			providerStatus: stripego.PaymentIntentStatusRequiresCapture,
			wantStatus:     flexsettlement.RecoveryPendingProvider,
		},
		{
			name:           "processing authorized funds",
			claim:          true,
			providerStatus: stripego.PaymentIntentStatusRequiresCapture,
			wantStatus:     flexsettlement.RecoveryReadyForCapture,
		},
		{
			name:           "pending captured funds",
			providerStatus: stripego.PaymentIntentStatusSucceeded,
			captured:       true,
			wantStatus:     flexsettlement.RecoveryCaptured,
		},
		{
			name:           "processing captured funds",
			claim:          true,
			providerStatus: stripego.PaymentIntentStatusSucceeded,
			captured:       true,
			wantStatus:     flexsettlement.RecoveryCaptured,
		},
		{
			name:           "provider still processing",
			claim:          true,
			providerStatus: stripego.PaymentIntentStatusProcessing,
			wantStatus:     flexsettlement.RecoveryPendingProvider,
		},
		{
			name:          "provider retrieval error",
			claim:         true,
			providerError: errors.New("simulated provider outage"),
			wantError:     true,
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
					"refusing recovery integration test on %q",
					databaseName,
				)
			}

			settlement, err :=
				postgres.CreateFlexSettlementFromCompletedTrip(
					ctx,
					db,
					lifecycle.TripID,
				)
			if err != nil {
				t.Fatalf("create settlement: %v", err)
			}

			if tt.claim {
				_, claimed, err :=
					postgres.ClaimFlexSettlementProcessing(
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
					Captured:          tt.captured,
				},
				err: tt.providerError,
			}

			service := flexsettlement.NewRecoveryService(
				db,
				verifier,
			)

			result, err := service.Recover(
				ctx,
				settlement.ID,
			)

			if tt.wantError {
				if err == nil {
					t.Fatal("expected recovery error")
				}
			} else {
				if err != nil {
					t.Fatalf("recover FLEX settlement: %v", err)
				}
				if result == nil ||
					result.Status != tt.wantStatus {
					t.Fatalf(
						"recovery status mismatch: %+v, want %s",
						result,
						tt.wantStatus,
					)
				}
			}

			if verifier.calls != 1 {
				t.Fatalf(
					"expected one provider verification, got %d",
					verifier.calls,
				)
			}

			if verifier.request.ProviderAuthorizationID !=
				lifecycle.ProviderID ||
				verifier.request.AuthorizationID !=
					lifecycle.AuthorizationID ||
				verifier.request.AmountCents !=
					lifecycle.AmountCents ||
				verifier.request.Currency !=
					lifecycle.Currency {
				t.Fatalf(
					"recovery verification identity mismatch: %+v",
					verifier.request,
				)
			}

			var persistedStatus string
			var paymentCount int

			if err := db.QueryRow(ctx, `
                SELECT status
                FROM flex_settlements
                WHERE id = $1
            `, settlement.ID).Scan(&persistedStatus); err != nil {
				t.Fatal(err)
			}

			expectedStatus := "PENDING"
			if tt.claim {
				expectedStatus = "PROCESSING"
			}

			if persistedStatus != expectedStatus {
				t.Fatalf(
					"recovery mutated settlement: got %s, want %s",
					persistedStatus,
					expectedStatus,
				)
			}

			if err := db.QueryRow(ctx, `
                SELECT COUNT(*)
                FROM payments
                WHERE trip_id = $1
            `, lifecycle.TripID).Scan(&paymentCount); err != nil {
				t.Fatal(err)
			}

			if paymentCount != 0 {
				t.Fatalf(
					"recovery created %d unexpected payments",
					paymentCount,
				)
			}
		})
	}
}
