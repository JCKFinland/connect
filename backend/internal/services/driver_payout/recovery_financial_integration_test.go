package driver_payout

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestRecoveryFinancialFixtureIntegration(t *testing.T) {
	fixture := newSubmissionFixture(t)

	ctx := context.Background()

	var driverID, companyID string

	err := fixture.db.QueryRow(ctx, `
        SELECT driver_id::text, company_id::text
        FROM driver_payouts
        WHERE id=$1::uuid
    `, fixture.payoutID).Scan(&driverID, &companyID)
	if err != nil {
		t.Fatal(err)
	}

	if uuid.Validate(driverID) != nil ||
		uuid.Validate(companyID) != nil {
		t.Fatal("invalid payout organizational identities")
	}

	organization := newRecoveryOrganizationFixture(
		t, fixture.db, companyID,
	)

	if organization.BranchID == "" ||
		organization.FleetID == "" ||
		organization.VehicleID == "" {
		t.Fatal("incomplete recovery organization fixture")
	}

	financial := newRecoveryFinancialFixture(
		t,
		fixture.db,
		fixture.payoutID,
		driverID,
		companyID,
	)

	if financial.payoutID != fixture.payoutID ||
		financial.earningID == "" ||
		financial.paymentID == "" ||
		financial.tripID == "" {
		t.Fatal("incomplete recovery financial fixture")
	}

	var gross, commission, net string
	var settlementStatus string
	var payoutStatus string
	var payoutItemCount int

	err = fixture.db.QueryRow(ctx, `
        SELECT
            gross_amount::numeric(12,2)::text,
            commission_amount::numeric(12,2)::text,
            net_amount::numeric(12,2)::text,
            settlement_status
        FROM driver_earnings
        WHERE id=$1::uuid
    `, financial.earningID).Scan(
		&gross, &commission, &net, &settlementStatus,
	)
	if err != nil {
		t.Fatal(err)
	}

	if gross != "100.00" ||
		commission != "7.00" ||
		net != "93.00" ||
		settlementStatus != "PROCESSING" {
		t.Fatalf(
			"unexpected earning: gross=%s commission=%s net=%s status=%s",
			gross, commission, net, settlementStatus,
		)
	}

	err = fixture.db.QueryRow(ctx, `
        SELECT payout_status
        FROM driver_payouts
        WHERE id=$1::uuid
    `, fixture.payoutID).Scan(&payoutStatus)
	if err != nil {
		t.Fatal(err)
	}

	if payoutStatus != "PENDING" {
		t.Fatalf("unexpected payout status: %s", payoutStatus)
	}

	err = fixture.db.QueryRow(ctx, `
        SELECT COUNT(*)
        FROM driver_payout_items
        WHERE payout_id=$1::uuid
          AND earning_id=$2::uuid
          AND amount=93
    `, fixture.payoutID, financial.earningID).
		Scan(&payoutItemCount)
	if err != nil {
		t.Fatal(err)
	}

	if payoutItemCount != 1 {
		t.Fatalf(
			"expected exactly one ?93 payout item, got %d",
			payoutItemCount,
		)
	}

	t.Log(
		"PASS: FLEX financial fixture gross=100 commission=7 net=93",
	)
	t.Log(
		"PASS: earning PROCESSING; payout PENDING; no finalization",
	)
}
