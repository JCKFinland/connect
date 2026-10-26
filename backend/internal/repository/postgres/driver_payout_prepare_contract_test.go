package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPrepareFlexDriverPayoutRejectsInvalidInput(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name   string
		params PrepareDriverPayoutParams
	}{
		{
			name: "missing driver",
			params: PrepareDriverPayoutParams{
				CompanyID:       uuid.NewString(),
				PayoutReference: "invalid-driver",
				PeriodStart:     now.Add(-time.Hour),
				PeriodEnd:       now,
			},
		},
		{
			name: "missing company",
			params: PrepareDriverPayoutParams{
				DriverID:        uuid.NewString(),
				PayoutReference: "invalid-company",
				PeriodStart:     now.Add(-time.Hour),
				PeriodEnd:       now,
			},
		},
		{
			name: "missing reference",
			params: PrepareDriverPayoutParams{
				DriverID:    uuid.NewString(),
				CompanyID:   uuid.NewString(),
				PeriodStart: now.Add(-time.Hour),
				PeriodEnd:   now,
			},
		},
		{
			name: "reversed period",
			params: PrepareDriverPayoutParams{
				DriverID:        uuid.NewString(),
				CompanyID:       uuid.NewString(),
				PayoutReference: "reversed-period",
				PeriodStart:     now,
				PeriodEnd:       now.Add(-time.Hour),
			},
		},
		{
			name: "zero period",
			params: PrepareDriverPayoutParams{
				DriverID:        uuid.NewString(),
				CompanyID:       uuid.NewString(),
				PayoutReference: "zero-period",
			},
		},
		{
			name: "reference too long",
			params: PrepareDriverPayoutParams{
				DriverID:        uuid.NewString(),
				CompanyID:       uuid.NewString(),
				PayoutReference: strings.Repeat("x", 101),
				PeriodStart:     now.Add(-time.Hour),
				PeriodEnd:       now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PrepareFlexDriverPayout(
				context.Background(),
				nil,
				tt.params,
			)

			if !errors.Is(err, ErrDriverPayoutInvalidInput) {
				t.Fatalf(
					"expected invalid input, got %v",
					err,
				)
			}
		})
	}
}

func TestPayoutEarningIDs(t *testing.T) {
	first := uuid.NewString()
	second := uuid.NewString()

	items := []payoutEligibleEarning{
		{id: first, gross: "10.00", net: "9.30"},
		{id: second, gross: "20.00", net: "18.60"},
	}

	ids := earningIDs(items)

	if len(ids) != 2 {
		t.Fatalf("expected 2 earning IDs, got %d", len(ids))
	}

	if ids[0] != first || ids[1] != second {
		t.Fatalf("unexpected earning IDs: %v", ids)
	}
}

func TestPayoutPreparationSourceFinancialGuards(t *testing.T) {
	source, err := os.ReadFile("driver_payout_prepare.go")
	if err != nil {
		t.Fatalf("read payout source: %v", err)
	}

	required := []string{
		"RunInTransaction(",
		"AcquireTransactionAdvisoryLock(",
		"JOIN flex_settlements fs",
		"fs.status = 'CAPTURED'",
		"p.status = 'PAID'",
		"t.status = 'COMPLETED'",
		"rr.matching_mode = 'FLEX'",
		"FOR UPDATE OF e",
		"NOT EXISTS (",
		"INSERT INTO driver_payouts",
		"INSERT INTO driver_payout_items",
		"settlement_status = 'PROCESSING'",
		"ErrDriverPayoutConflict",
	}

	text := string(source)

	for _, guard := range required {
		if !strings.Contains(text, guard) {
			t.Errorf("missing payout financial guard: %s", guard)
		}
	}

	forbidden := []string{
		"stripe.New(",
		"CreateTransfer(",
		"CreatePayout(",
	}

	for _, operation := range forbidden {
		if strings.Contains(text, operation) {
			t.Errorf(
				"external payout operation must not occur: %s",
				operation,
			)
		}
	}
}

func TestPayoutPreparationCommissionIntegrity(t *testing.T) {
	source, err := os.ReadFile("driver_payout_prepare.go")
	if err != nil {
		t.Fatalf("read payout source: %v", err)
	}

	required := []string{
		"AND e.commission_amount = ROUND(e.gross_amount * 0.07, 2)",
		"AND e.net_amount = e.gross_amount - e.commission_amount",
		"AND e.bonus_amount = 0",
		"AND e.tip_amount = 0",
		"AND e.adjustment_amount = 0",
		"AND e.tax_withheld = 0",
	}

	for _, condition := range required {
		if !strings.Contains(string(source), condition) {
			t.Errorf("missing financial invariant: %s", condition)
		}
	}
}
