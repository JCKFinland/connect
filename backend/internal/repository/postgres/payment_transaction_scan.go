package postgres

import "github.com/JCKFinland/connect/backend/internal/models"

type paymentTransactionScanner interface {
	Scan(dest ...any) error
}

func scanPaymentTransaction(
	scanner paymentTransactionScanner,
) (*models.PaymentTransaction, error) {
	var transaction models.PaymentTransaction

	err := scanner.Scan(
		&transaction.ID,
		&transaction.PaymentID,
		&transaction.TransactionReference,
		&transaction.Provider,
		&transaction.ProviderTransactionID,
		&transaction.IdempotencyKey,
		&transaction.TransactionType,
		&transaction.Status,
		&transaction.Amount,
		&transaction.Currency,
		&transaction.GatewayRequest,
		&transaction.GatewayResponse,
		&transaction.ProcessedAt,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &transaction, nil
}
