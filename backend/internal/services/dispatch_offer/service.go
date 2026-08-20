package dispatch_offer

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

const (
	StatusPending   = "PENDING"
	StatusAccepted  = "ACCEPTED"
	StatusRejected  = "REJECTED"
	StatusExpired   = "EXPIRED"
	StatusCancelled = "CANCELLED"

	defaultOfferTimeout = 30 * time.Second
)

type Service struct {
	db           *pgxpool.Pool
	repo         repository.DispatchOfferRepository
	offerTimeout time.Duration
}

type Dependencies struct {
	DB           *pgxpool.Pool
	Offers       repository.DispatchOfferRepository
	OfferTimeout time.Duration
}

func NewService(
	deps Dependencies,
) *Service {

	timeout := deps.OfferTimeout

	if timeout <= 0 {
		timeout = defaultOfferTimeout
	}

	return &Service{
		db:           deps.DB,
		repo:         deps.Offers,
		offerTimeout: timeout,
	}
}

// Create creates a new PENDING dispatch offer.
func (s *Service) Create(
	ctx context.Context,
	offer *models.DispatchOffer,
) error {

	if offer == nil {
		return fmt.Errorf("dispatch offer is required")
	}

	if offer.RideRequestID == "" {
		return fmt.Errorf("ride request ID is required")
	}

	if offer.DriverID == "" {
		return fmt.Errorf("driver ID is required")
	}

	if offer.VehicleID == "" {
		return fmt.Errorf("vehicle ID is required")
	}

	if offer.CompanyID == "" {
		return fmt.Errorf("company ID is required")
	}

	if offer.BranchID == "" {
		return fmt.Errorf("branch ID is required")
	}

	if offer.FleetID == "" {
		return fmt.Errorf("fleet ID is required")
	}

	now := time.Now().UTC()

	if offer.ID == "" {
		offer.ID = uuid.NewString()
	}

	if offer.Status == "" {
		offer.Status = StatusPending
	}

	if offer.Status != StatusPending {
		return fmt.Errorf(
			"new dispatch offer must have status %s",
			StatusPending,
		)
	}

	if offer.OfferedAt.IsZero() {
		offer.OfferedAt = now
	}

	if offer.ExpiresAt.IsZero() {
		offer.ExpiresAt = offer.OfferedAt.Add(
			s.offerTimeout,
		)
	}

	if !offer.ExpiresAt.After(offer.OfferedAt) {
		return fmt.Errorf(
			"dispatch offer expiry must be after offered time",
		)
	}

	if offer.CreatedAt.IsZero() {
		offer.CreatedAt = now
	}

	if offer.UpdatedAt.IsZero() {
		offer.UpdatedAt = now
	}

	return s.repo.Create(
		ctx,
		offer,
	)
}

// GetByID retrieves a dispatch offer.
func (s *Service) GetByID(
	ctx context.Context,
	id string,
) (*models.DispatchOffer, error) {

	if id == "" {
		return nil, fmt.Errorf(
			"dispatch offer ID is required",
		)
	}

	return s.repo.GetByID(
		ctx,
		id,
	)
}

// GetPendingByDriver retrieves a driver's active offer.
func (s *Service) GetPendingByDriver(
	ctx context.Context,
	driverID string,
) (*models.DispatchOffer, error) {

	if driverID == "" {
		return nil, fmt.Errorf(
			"driver ID is required",
		)
	}

	return s.repo.GetPendingByDriver(
		ctx,
		driverID,
	)
}

// GetPendingByRideRequest retrieves the active offer for a ride request.
func (s *Service) GetPendingByRideRequest(
	ctx context.Context,
	rideRequestID string,
) (*models.DispatchOffer, error) {

	if rideRequestID == "" {
		return nil, fmt.Errorf(
			"ride request ID is required",
		)
	}

	return s.repo.GetPendingByRideRequest(
		ctx,
		rideRequestID,
	)
}
