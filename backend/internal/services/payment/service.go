package payment

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service defines payment business operations.
type Service interface {
	CreateForCompletedTrip(
		ctx context.Context,
		tripID string,
		paymentMethod string,
	) (*models.Payment, error)

	GetByID(
		ctx context.Context,
		id string,
	) (*models.Payment, error)

	GetByTripID(
		ctx context.Context,
		tripID string,
	) (*models.Payment, error)

	UpdateStatus(
		ctx context.Context,
		id string,
		status string,
	) (*models.Payment, error)
}

// Dependencies contains payment service dependencies.
type Dependencies struct {
	DB *pgxpool.Pool

	Payments repository.PaymentRepository
}

// paymentService implements Service.
type paymentService struct {
	db *pgxpool.Pool

	payments repository.PaymentRepository
}

// NewService creates a payment service.
func NewService(
	deps Dependencies,
) Service {
	return &paymentService{
		db:       deps.DB,
		payments: deps.Payments,
	}
}

var _ Service = (*paymentService)(nil)
