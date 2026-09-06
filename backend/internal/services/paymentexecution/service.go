package paymentexecution

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type Result struct {
	Transaction *models.PaymentTransaction

	// ClientSecret is an ephemeral provider credential used by the
	// customer application to complete provider-side authentication.
	//
	// It must not be logged or persisted.
	ClientSecret string

	RequiresCustomerAction bool
}

type Service interface {
	Execute(
		ctx context.Context,
		transactionID string,
	) (*Result, error)
}
