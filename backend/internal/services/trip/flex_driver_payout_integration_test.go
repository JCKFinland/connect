package trip

import (
	"context"
	"errors"
	"math/big"
	"os"
	"testing"
	"time"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func TestFlexDriverPayoutPreparationFromCapturedTrip(t *testing.T) {
	if os.Getenv("CONNECT_REQUIRE_SETTLEMENT_TEST_DB") != "1" ||
		os.Getenv("DB_NAME") != "connect_settlement_test" {
		t.Skip("requires isolated FLEX settlement test database")
	}

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
		t.Fatalf(
			"refusing payout integration test on %q",
			databaseName,
		)
	}

	if lifecycle.AmountCents != 2328 ||
		lifecycle.Currency != "EUR" {
		t.Fatalf(
			"unexpected FLEX contract: cents=%d currency=%s",
			lifecycle.AmountCents,
			lifecycle.Currency,
		)
	}

	settlement, err :=
		postgresrepo.CreateFlexSettlementFromCompletedTrip(
			ctx,
			db,
			lifecycle.TripID,
		)
	if err != nil {
		t.Fatalf("create FLEX settlement: %v", err)
	}

	_, claimed, err :=
		postgresrepo.ClaimFlexSettlementProcessing(
			ctx,
			db,
			settlement.ID,
		)
	if err != nil || !claimed {
		t.Fatalf(
			"claim FLEX settlement: claimed=%v err=%v",
			claimed,
			err,
		)
	}

	preparedCapture, err :=
		postgresrepo.PrepareFlexSettlementCapture(
			ctx,
			db,
			settlement.ID,
		)
	if err != nil {
		t.Fatalf("prepare FLEX capture: %v", err)
	}

	if preparedCapture == nil ||
		preparedCapture.PaymentID == "" ||
		preparedCapture.CaptureTransactionID == "" {
		t.Fatalf(
			"invalid capture preparation: %+v",
			preparedCapture,
		)
	}

	if _, err :=
		postgresrepo.MarkFlexSettlementCaptureRequested(
			ctx,
			db,
			settlement.ID,
		); err != nil {
		t.Fatalf("mark capture requested: %v", err)
	}

	// This provider identity is synthetic. No Stripe API is called.
	// Finalization is exercised only against the isolated test DB.
	providerCaptureID :=
		preparedCapture.ProviderAuthorizationID

	if providerCaptureID == "" {
		t.Fatal("missing synthetic provider identity")
	}

	if err := postgresrepo.FinalizeFlexSettlementCapture(
		ctx,
		db,
		settlement.ID,
		providerCaptureID,
	); err != nil {
		t.Fatalf("finalize simulated capture: %v", err)
	}

	var driverID, companyID string
	var earningID string
	var gross, commission, net string
	var earningStatus string

	err = db.QueryRow(ctx, `
        SELECT
            e.id,
            e.driver_id,
            e.company_id,
            e.gross_amount::text,
            e.commission_amount::text,
            e.net_amount::text,
            e.settlement_status
        FROM driver_earnings e
        WHERE e.trip_id = $1
    `, lifecycle.TripID).Scan(
		&earningID,
		&driverID,
		&companyID,
		&gross,
		&commission,
		&net,
		&earningStatus,
	)
	if err != nil {
		t.Fatalf("read captured FLEX earning: %v", err)
	}

	if !postgresrepoAmountsEqualForPayoutTest(gross, "23.28") ||
		!postgresrepoAmountsEqualForPayoutTest(commission, "1.63") ||
		!postgresrepoAmountsEqualForPayoutTest(net, "21.65") ||
		earningStatus != "PENDING" {
		t.Fatalf(
			"incorrect captured earning: gross=%s commission=%s net=%s status=%s",
			gross,
			commission,
			net,
			earningStatus,
		)
	}

	reference := "connect-flex-payout-" + uuid.NewString()

	// Registered after the trip cleanup, so it runs first.
	// Remove payout items before the payout and earning records.
	t.Cleanup(func() {
		cleanupCtx := context.Background()

		if _, err := db.Exec(cleanupCtx, `
            DELETE FROM driver_payout_items
            WHERE payout_id IN (
                SELECT id FROM driver_payouts
                WHERE payout_reference = $1
            )
        `, reference); err != nil {
			t.Errorf("cleanup payout items: %v", err)
		}

		if _, err := db.Exec(cleanupCtx, `
            DELETE FROM driver_payouts
            WHERE payout_reference = $1
        `, reference); err != nil {
			t.Errorf("cleanup payout: %v", err)
		}
	})

	now := time.Now().UTC()

	params := postgresrepo.PrepareDriverPayoutParams{
		DriverID:        driverID,
		CompanyID:       companyID,
		PayoutReference: reference,
		PeriodStart:     now.Add(-24 * time.Hour),
		PeriodEnd:       now.Add(time.Hour),
	}

	payout, err := postgresrepo.PrepareFlexDriverPayout(
		ctx,
		db,
		params,
	)
	if err != nil {
		t.Fatalf("prepare FLEX driver payout: %v", err)
	}

	if payout == nil {
		t.Fatal("nil prepared payout")
	}

	if payout.DriverID != driverID ||
		payout.CompanyID != companyID ||
		payout.PayoutReference != reference ||
		payout.Currency != "EUR" ||
		payout.PayoutMethod != "BANK_TRANSFER" ||
		payout.PayoutStatus != "PENDING" ||
		payout.ItemCount != 1 ||
		!postgresrepoAmountsEqualForPayoutTest(
			payout.GrossAmount, "23.28",
		) ||
		!postgresrepoAmountsEqualForPayoutTest(
			payout.NetAmount, "21.65",
		) {
		t.Fatalf("invalid prepared payout: %+v", payout)
	}

	var itemCount int64
	var itemAmount, claimedStatus string

	err = db.QueryRow(ctx, `
        SELECT
            COUNT(*),
            COALESCE(MAX(dpi.amount)::text, '0'),
            COALESCE(MAX(e.settlement_status), '')
        FROM driver_payout_items dpi
        JOIN driver_earnings e
            ON e.id = dpi.earning_id
        WHERE dpi.payout_id = $1
          AND dpi.earning_id = $2
    `, payout.ID, earningID).Scan(
		&itemCount,
		&itemAmount,
		&claimedStatus,
	)
	if err != nil {
		t.Fatalf("read payout item: %v", err)
	}

	if itemCount != 1 ||
		!postgresrepoAmountsEqualForPayoutTest(
			itemAmount, "21.65",
		) ||
		claimedStatus != "PROCESSING" {
		t.Fatalf(
			"incorrect payout claim: count=%d amount=%s status=%s",
			itemCount,
			itemAmount,
			claimedStatus,
		)
	}

	replay, err := postgresrepo.PrepareFlexDriverPayout(
		ctx,
		db,
		params,
	)
	if err != nil {
		t.Fatalf("replay prepared payout: %v", err)
	}

	if replay == nil ||
		replay.ID != payout.ID ||
		replay.ItemCount != 1 {
		t.Fatalf(
			"payout replay created inconsistent result: %+v",
			replay,
		)
	}

	conflictParams := params
	conflictParams.PeriodEnd = params.PeriodEnd.Add(time.Hour)

	conflicting, err := postgresrepo.PrepareFlexDriverPayout(
		ctx,
		db,
		conflictParams,
	)
	if !errors.Is(
		err,
		postgresrepo.ErrDriverPayoutConflict,
	) || conflicting != nil {
		t.Fatalf(
			"expected payout reference conflict: result=%+v err=%v",
			conflicting,
			err,
		)
	}

	var totalPayouts, totalItems int64

	err = db.QueryRow(ctx, `
        SELECT
            (SELECT COUNT(*) FROM driver_payouts
             WHERE payout_reference = $1),
            (SELECT COUNT(*) FROM driver_payout_items
             WHERE earning_id = $2)
    `, reference, earningID).Scan(
		&totalPayouts,
		&totalItems,
	)
	if err != nil {
		t.Fatalf("count financial records: %v", err)
	}

	if totalPayouts != 1 || totalItems != 1 {
		t.Fatalf(
			"duplicate payout records: payouts=%d items=%d",
			totalPayouts,
			totalItems,
		)
	}

	t.Logf(
		"FLEX payout verified: gross=23.28 commission=1.63 net=21.65 payout=%s",
		payout.ID,
	)
}

func postgresrepoAmountsEqualForPayoutTest(
	actual, expected string,
) bool {
	actualValue, ok := new(big.Rat).SetString(actual)
	if !ok {
		return false
	}

	expectedValue, ok := new(big.Rat).SetString(expected)
	if !ok {
		return false
	}

	return actualValue.Cmp(expectedValue) == 0
}
