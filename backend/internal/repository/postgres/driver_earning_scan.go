package postgres

import "github.com/JCKFinland/connect/backend/internal/models"

type driverEarningScanner interface {
	Scan(dest ...any) error
}

func scanDriverEarning(
	scanner driverEarningScanner,
) (*models.DriverEarning, error) {
	var earning models.DriverEarning

	err := scanner.Scan(
		&earning.ID,
		&earning.TripID,
		&earning.DriverID,
		&earning.CompanyID,
		&earning.FareID,
		&earning.PaymentID,
		&earning.GrossAmount,
		&earning.CommissionAmount,
		&earning.BonusAmount,
		&earning.TipAmount,
		&earning.AdjustmentAmount,
		&earning.TaxWithheld,
		&earning.NetAmount,
		&earning.Currency,
		&earning.SettlementStatus,
		&earning.CalculatedAt,
		&earning.SettledAt,
		&earning.CreatedAt,
		&earning.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &earning, nil
}
