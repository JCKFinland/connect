package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// DriverEarningRepository defines persistence operations for
// authoritative driver earnings.
type DriverEarningRepository interface {
	// CreateFromPaidPayment creates the driver's earning directly from
	// the authoritative paid payment, completed trip, and trip fare.
	//
	// Driver, company, fare, gross amount, currency, and payment identity
	// are derived by PostgreSQL and are never supplied by the caller.
	CreateFromPaidPayment(
		ctx context.Context,
		paymentID string,
	) (*models.DriverEarning, error)

	GetByTripID(
		ctx context.Context,
		tripID string,
	) (*models.DriverEarning, error)

	GetSummaryByDriverID(
		ctx context.Context,
		driverID string,
	) (*models.DriverEarningsSummary, error)

	ListRecentByDriverID(
		ctx context.Context,
		driverID string,
		limit int,
	) ([]models.DriverEarning, error)
}
