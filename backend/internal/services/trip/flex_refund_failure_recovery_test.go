package trip

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
	"github.com/google/uuid"
)

func TestFlexFailedOrCancelledRefundRestoresPayoutEligibility(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("DB_NAME") != "connect_settlement_test" {
		t.Skip("requires isolated FLEX settlement test database")
	}

	for _, terminalStatus := range []string{
		paymenttransaction.StatusFailed,
		paymenttransaction.StatusCancelled,
	} {
		t.Run(terminalStatus, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(), 45*time.Second,
			)
			defer cancel()

			lifecycle := completeAuthorizedFlexLifecycle(t)
			db := lifecycle.Fixture.DB

			var databaseName string
			if err := db.QueryRow(
				ctx,
				"SELECT current_database()",
			).Scan(&databaseName); err != nil {
				t.Fatal(err)
			}
			if databaseName != "connect_settlement_test" {
				t.Fatalf("unsafe database: %s", databaseName)
			}

			settlement, err := postgresrepo.CreateFlexSettlementFromCompletedTrip(
				ctx, db, lifecycle.TripID,
			)
			if err != nil {
				t.Fatalf("create settlement: %v", err)
			}

			_, claimed, err := postgresrepo.ClaimFlexSettlementProcessing(
				ctx, db, settlement.ID,
			)
			if err != nil || !claimed {
				t.Fatalf("claim settlement: claimed=%v err=%v", claimed, err)
			}

			capture, err := postgresrepo.PrepareFlexSettlementCapture(
				ctx, db, settlement.ID,
			)
			if err != nil {
				t.Fatalf("prepare capture: %v", err)
			}
			if capture == nil || capture.PaymentID == "" {
				t.Fatal("missing captured payment ID")
			}

			if _, err := postgresrepo.MarkFlexSettlementCaptureRequested(
				ctx, db, settlement.ID,
			); err != nil {
				t.Fatalf("mark capture requested: %v", err)
			}

			if err := postgresrepo.FinalizeFlexSettlementCapture(
				ctx, db, settlement.ID, capture.ProviderAuthorizationID,
			); err != nil {
				t.Fatalf("finalize capture: %v", err)
			}

			var driverID, companyID, earningID, earningPaymentID string
			var earningStatus string

			if err := db.QueryRow(ctx, `
                SELECT e.id, e.driver_id, e.company_id,
                       e.payment_id::text, e.settlement_status
                FROM driver_earnings e
                WHERE e.trip_id = $1
            `, lifecycle.TripID).Scan(
				&earningID,
				&driverID,
				&companyID,
				&earningPaymentID,
				&earningStatus,
			); err != nil {
				t.Fatalf("read earning: %v", err)
			}

			if earningPaymentID != capture.PaymentID ||
				earningStatus != "PENDING" {
				t.Fatalf(
					"unexpected earning: payment=%s status=%s",
					earningPaymentID, earningStatus,
				)
			}

			service := paymenttransaction.NewService(db)

			refund, err := service.InitiateOperation(
				ctx,
				paymenttransaction.InitiateOperationRequest{
					PaymentID:       capture.PaymentID,
					Provider:        "TEST_PROVIDER",
					IdempotencyKey:  uuid.NewString(),
					TransactionType: paymenttransaction.TypeRefund,
					Amount:          "0.40",
				},
			)
			if err != nil {
				t.Fatalf("initiate refund: %v", err)
			}
			if refund == nil ||
				refund.Status != paymenttransaction.StatusPending {
				t.Fatalf("expected pending refund, got %+v", refund)
			}

			now := time.Now().UTC()
			periodStart := now.Add(-24 * time.Hour)
			periodEnd := now.Add(time.Hour)

			pendingReference := "connect-refund-pending-" + uuid.NewString()
			recoveryReference := "connect-refund-recovery-" + uuid.NewString()
			duplicateReference := "connect-refund-duplicate-" + uuid.NewString()

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(
					context.Background(), 10*time.Second,
				)
				defer cleanupCancel()

				for _, reference := range []string{
					pendingReference,
					recoveryReference,
					duplicateReference,
				} {
					_, _ = db.Exec(cleanupCtx, `
                        DELETE FROM driver_payout_items
                        WHERE payout_id IN (
                            SELECT id FROM driver_payouts
                            WHERE payout_reference = $1
                        )
                    `, reference)

					_, _ = db.Exec(cleanupCtx, `
                        DELETE FROM driver_payouts
                        WHERE payout_reference = $1
                    `, reference)
				}
			})

			prepare := func(reference string) (
				*postgresrepo.PreparedDriverPayout, error,
			) {
				return postgresrepo.PrepareFlexDriverPayout(
					ctx,
					db,
					postgresrepo.PrepareDriverPayoutParams{
						DriverID:        driverID,
						CompanyID:       companyID,
						PayoutReference: reference,
						PeriodStart:     periodStart,
						PeriodEnd:       periodEnd,
					},
				)
			}

			pendingPayout, err := prepare(pendingReference)
			if !errors.Is(
				err, postgresrepo.ErrDriverPayoutNoEligibleEarnings,
			) {
				t.Fatalf(
					"pending refund did not block payout: payout=%+v err=%v",
					pendingPayout, err,
				)
			}

			result, err := service.ApplyResult(
				ctx,
				refund.ID,
				paymenttransaction.ApplyResultRequest{
					Status: terminalStatus,
				},
			)
			if err != nil {
				t.Fatalf("apply refund terminal status: %v", err)
			}
			if result == nil || result.Status != terminalStatus {
				t.Fatalf(
					"expected refund %s, got %+v",
					terminalStatus, result,
				)
			}

			var paymentStatus string
			if err := db.QueryRow(ctx, `
                SELECT status FROM payments WHERE id = $1
            `, capture.PaymentID).Scan(&paymentStatus); err != nil {
				t.Fatalf("read aggregate payment: %v", err)
			}
			if paymentStatus != "PAID" {
				t.Fatalf(
					"expected PAID after %s refund, got %s",
					terminalStatus, paymentStatus,
				)
			}

			payout, err := prepare(recoveryReference)
			if err != nil {
				t.Fatalf(
					"prepare payout after %s refund: %v",
					terminalStatus, err,
				)
			}
			if payout == nil || payout.ItemCount != 1 {
				t.Fatalf(
					"expected one payout item after %s, got %+v",
					terminalStatus, payout,
				)
			}

			var payoutItems int
			if err := db.QueryRow(ctx, `
                SELECT COUNT(*)
                FROM driver_payout_items
                WHERE earning_id = $1 AND payout_id = $2
            `, earningID, payout.ID).Scan(&payoutItems); err != nil {
				t.Fatalf("count payout items: %v", err)
			}
			if payoutItems != 1 {
				t.Fatalf(
					"expected exactly one payout claim, got %d",
					payoutItems,
				)
			}

			if err := db.QueryRow(ctx, `
                SELECT settlement_status
                FROM driver_earnings
                WHERE id = $1
            `, earningID).Scan(&earningStatus); err != nil {
				t.Fatalf("read claimed earning: %v", err)
			}
			if earningStatus != "PROCESSING" {
				t.Fatalf(
					"expected PROCESSING earning, got %s",
					earningStatus,
				)
			}

			duplicatePayout, err := prepare(duplicateReference)
			if !errors.Is(
				err, postgresrepo.ErrDriverPayoutNoEligibleEarnings,
			) {
				t.Fatalf(
					"duplicate earning claim allowed: payout=%+v err=%v",
					duplicatePayout, err,
				)
			}
		})
	}
}
