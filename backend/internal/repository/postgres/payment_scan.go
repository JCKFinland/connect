package postgres

import "github.com/JCKFinland/connect/backend/internal/models"

type paymentScanner interface {
	Scan(dest ...any) error
}

func scanPayment(
	scanner paymentScanner,
) (*models.Payment, error) {
	var payment models.Payment

	err := scanner.Scan(
		&payment.ID,
		&payment.TripID,
		&payment.FareID,
		&payment.CustomerID,
		&payment.Status,
		&payment.PaymentMethod,
		&payment.Amount,
		&payment.Currency,
		&payment.PaidAt,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &payment, nil
}
