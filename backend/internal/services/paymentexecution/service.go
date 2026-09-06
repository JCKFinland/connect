package paymentexecution

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type Service interface {
	Execute(
		ctx context.Context,
		transactionID string,
	) (*models.PaymentTransaction, error)
}
