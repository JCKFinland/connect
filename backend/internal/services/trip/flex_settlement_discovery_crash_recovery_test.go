package trip

import (
	"context"
	"os"
	"testing"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexsettlement "github.com/JCKFinland/connect/backend/internal/services/flex_settlement"
)

func TestFlexSettlementDiscoveryCrashRecovery(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" {
		t.Skip("requires isolated FLEX settlement database")
	}
	if os.Getenv("FLEX_SETTLEMENT_WORKER_ENABLED") != "false" {
		t.Fatal("background settlement worker must remain disabled")
	}

	cases := []struct {
		name     string
		captured bool
	}{
		{"provider confirms capture", true},
		{"provider has not confirmed capture", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
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
				t.Fatalf("unsafe test database: %q", databaseName)
			}

			settlement, err :=
				postgresrepo.CreateFlexSettlementFromCompletedTrip(
					ctx, db, lifecycle.TripID,
				)
			if err != nil {
				t.Fatal(err)
			}

			_, claimed, err :=
				postgresrepo.ClaimFlexSettlementProcessing(
					ctx, db, settlement.ID,
				)
			if err != nil || !claimed {
				t.Fatalf(
					"claim settlement: claimed=%v err=%v",
					claimed, err,
				)
			}

			if _, err := postgresrepo.PrepareFlexSettlementCapture(
				ctx, db, settlement.ID,
			); err != nil {
				t.Fatalf("prepare capture: %v", err)
			}

			if _, err := postgresrepo.MarkFlexSettlementCaptureRequested(
				ctx, db, settlement.ID,
			); err != nil {
				t.Fatalf("persist capture request marker: %v", err)
			}

			var marked bool
			if err := db.QueryRow(ctx, `
                SELECT capture_requested_at IS NOT NULL
                FROM flex_settlements
                WHERE id = $1
            `, settlement.ID).Scan(&marked); err != nil {
				t.Fatal(err)
			}
			if !marked {
				t.Fatal("capture marker was not committed")
			}

			// Simulate restart: create fresh worker and orchestration
			// objects against the already-persisted database state.
			verifier := &statefulFlexCaptureVerifier{
				providerID: lifecycle.ProviderID,
				captured:   tc.captured,
			}
			capture := &recordingFlexCaptureExecutor{}

			orchestrator := flexsettlement.NewOrchestrator(
				db,
				flexsettlement.NewRecoveryService(db, verifier),
			)
			dispatcher, err :=
				flexsettlement.NewDiscoveryDispatcher(
					orchestrator, capture,
				)
			if err != nil {
				t.Fatal(err)
			}

			scheduler, err :=
				flexsettlement.NewDiscoveryScheduler(db, 200)
			if err != nil {
				t.Fatal(err)
			}

			worker, err := flexsettlement.NewDiscoveryWorker(
				scheduler, dispatcher,
			)
			if err != nil {
				t.Fatal(err)
			}

			candidates, err := scheduler.Next(ctx)
			if err != nil {
				t.Fatal(err)
			}

			kind, found := findFlexSettlementCandidate(
				candidates, settlement.ID,
			)
			if !found ||
				kind != postgresrepo.FlexSettlementCandidateRecovery {
				t.Fatalf(
					"marked settlement must be RECOVERY: found=%v kind=%q",
					found, kind,
				)
			}

			run := func() {
				t.Helper()
				if err := worker.RunOnce(ctx, func(err error) {
					t.Errorf("worker dispatch: %v", err)
				}); err != nil {
					t.Fatalf("worker run: %v", err)
				}
			}

			run()

			if capture.calls != 0 {
				t.Fatalf(
					"recovery submitted provider capture: %d",
					capture.calls,
				)
			}

			if tc.captured {
				assertCapturedFlexOrchestratorState(
					t, ctx, db, settlement.ID,
					lifecycle.TripID,
					lifecycle.ProviderID,
					lifecycle.AmountCents,
				)
			} else {
				var status string
				var markerExists bool
				var paidCount int
				var earningCount int
				var successfulCaptures int

				err := db.QueryRow(ctx, `
                    SELECT
                        fs.status,
                        fs.capture_requested_at IS NOT NULL,
                        (
                            SELECT COUNT(*)
                            FROM payments p
                            WHERE p.trip_id = fs.trip_id
                              AND p.status = 'PAID'
                        ),
                        (
                            SELECT COUNT(*)
                            FROM driver_earnings de
                            WHERE de.trip_id = fs.trip_id
                        ),
                        (
                            SELECT COUNT(*)
                            FROM payment_transactions pt
                            JOIN payments p ON p.id = pt.payment_id
                            WHERE p.trip_id = fs.trip_id
                              AND pt.transaction_type = 'CAPTURE'
                              AND pt.status = 'SUCCESS'
                        )
                    FROM flex_settlements fs
                    WHERE fs.id = $1
                `, settlement.ID).Scan(
					&status,
					&markerExists,
					&paidCount,
					&earningCount,
					&successfulCaptures,
				)
				if err != nil {
					t.Fatal(err)
				}

				if status != "PROCESSING" ||
					!markerExists ||
					paidCount != 0 ||
					earningCount != 0 ||
					successfulCaptures != 0 {
					t.Fatalf(
						"unsafe unresolved state: status=%s marker=%t paid=%d earnings=%d captures=%d",
						status, markerExists, paidCount,
						earningCount, successfulCaptures,
					)
				}
			}

			// A second polling pass must not create another
			// provider request or duplicate financial records.
			run()

			if capture.calls != 0 {
				t.Fatalf(
					"duplicate provider capture after replay: %d",
					capture.calls,
				)
			}

			if tc.captured {
				assertCapturedFlexOrchestratorState(
					t, ctx, db, settlement.ID,
					lifecycle.TripID,
					lifecycle.ProviderID,
					lifecycle.AmountCents,
				)
			}
		})
	}
}
