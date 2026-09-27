package postgres

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func (r *DriverEarningRepository) GetSummaryByDriverID(
	ctx context.Context,
	driverID string,
) (*models.DriverEarningsSummary, error) {
	const query = `
		SELECT
			COALESCE(SUM(gross_amount), 0)::text,
			COALESCE(SUM(commission_amount), 0)::text,
			COALESCE(SUM(bonus_amount), 0)::text,
			COALESCE(SUM(tip_amount), 0)::text,
			COALESCE(SUM(adjustment_amount), 0)::text,
			COALESCE(SUM(tax_withheld), 0)::text,
			COALESCE(SUM(net_amount), 0)::text,
			COALESCE(MAX(currency), 'EUR'),
			COUNT(*)
		FROM driver_earnings
		WHERE driver_id = $1
	`

	var summary models.DriverEarningsSummary

	err := r.db.QueryRow(
		ctx,
		query,
		driverID,
	).Scan(
		&summary.GrossAmount,
		&summary.CommissionAmount,
		&summary.BonusAmount,
		&summary.TipAmount,
		&summary.AdjustmentAmount,
		&summary.TaxWithheld,
		&summary.NetAmount,
		&summary.Currency,
		&summary.TripCount,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get driver earnings summary: %w",
			err,
		)
	}

	return &summary, nil
}
