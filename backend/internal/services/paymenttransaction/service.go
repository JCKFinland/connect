package paymenttransaction

import (
	"context"
	"encoding/json"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApplyResultRequest struct {
	Status string

	ProviderTransactionID *string

	GatewayResponse json.RawMessage
}

type Service interface {
	ApplyResult(
		ctx context.Context,
		transactionID string,
		req ApplyResultRequest,
	) (*models.PaymentTransaction, error)
}

type paymentTransactionService struct {
	db *pgxpool.Pool
}

func NewService(
	db *pgxpool.Pool,
) Service {
	return &paymentTransactionService{
		db: db,
	}
}

var _ Service = (*paymentTransactionService)(nil)
