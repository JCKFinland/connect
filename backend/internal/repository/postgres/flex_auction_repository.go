package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FlexAuctionRepository implements repository.FlexAuctionRepository.
type FlexAuctionRepository struct {
	db DBTX
}

var _ repository.FlexAuctionRepository = (*FlexAuctionRepository)(nil)

func NewFlexAuctionRepository(
	db *pgxpool.Pool,
) repository.FlexAuctionRepository {
	return &FlexAuctionRepository{db: db}
}

func NewFlexAuctionRepositoryWithDB(
	db DBTX,
) repository.FlexAuctionRepository {
	return &FlexAuctionRepository{db: db}
}

// Create persists a newly opened auction.
// Lifecycle transitions must use dedicated transactional operations.
func (r *FlexAuctionRepository) Create(
	ctx context.Context,
	auction *models.FlexAuction,
) error {
	if auction == nil {
		return errors.New("flex auction is required")
	}
	if auction.RideRequestID == "" {
		return errors.New("flex ride request ID is required")
	}

	amount, err := flexCentsToDecimal(auction.PassengerOfferCents)
	if err != nil {
		return fmt.Errorf("invalid flex passenger offer: %w", err)
	}

	if auction.Currency != "EUR" {
		return errors.New("flex auction currency must be EUR")
	}

	if auction.ID == "" {
		auction.ID = uuid.NewString()
	}

	now := time.Now().UTC()

	if auction.OpenedAt.IsZero() {
		auction.OpenedAt = now
	}
	if !auction.ExpiresAt.After(auction.OpenedAt) {
		return errors.New("flex auction expiry must follow opening")
	}

	auction.Status = models.FlexAuctionStatusOpen
	auction.SelectedOfferID = nil
	auction.ResolvedAt = nil

	if auction.CreatedAt.IsZero() {
		auction.CreatedAt = now
	}
	auction.UpdatedAt = now

	const query = `
		INSERT INTO flex_auctions (
			id,
			ride_request_id,
			passenger_offer_amount,
			currency,
			status,
			selected_offer_id,
			opened_at,
			expires_at,
			resolved_at,
			created_at,
			updated_at
		)
		VALUES (
			$1, $2, $3::numeric, $4, 'OPEN',
			NULL, $5, $6, NULL, $7, $8
		)
	`

	_, err = r.db.Exec(
		ctx,
		query,
		auction.ID,
		auction.RideRequestID,
		amount,
		auction.Currency,
		auction.OpenedAt,
		auction.ExpiresAt,
		auction.CreatedAt,
		auction.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create flex auction: %w", err)
	}

	return nil
}

const flexAuctionColumns = `
	id,
	ride_request_id,
	passenger_offer_amount::text,
	currency,
	status,
	selected_offer_id,
	opened_at,
	expires_at,
	resolved_at,
	created_at,
	updated_at
`

func (r *FlexAuctionRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.FlexAuction, error) {
	return r.getOne(
		ctx,
		"SELECT "+flexAuctionColumns+" FROM flex_auctions WHERE id = $1",
		id,
	)
}

func (r *FlexAuctionRepository) GetByRideRequestID(
	ctx context.Context,
	rideRequestID string,
) (*models.FlexAuction, error) {
	return r.getOne(
		ctx,
		"SELECT "+flexAuctionColumns+
			" FROM flex_auctions WHERE ride_request_id = $1",
		rideRequestID,
	)
}

// GetByIDForUpdate must be called with a transaction-bound repository.
func (r *FlexAuctionRepository) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.FlexAuction, error) {
	return r.getOne(
		ctx,
		"SELECT "+flexAuctionColumns+
			" FROM flex_auctions WHERE id = $1 FOR UPDATE",
		id,
	)
}

func (r *FlexAuctionRepository) getOne(
	ctx context.Context,
	query string,
	id string,
) (*models.FlexAuction, error) {
	auction := &models.FlexAuction{}
	var amount string

	err := r.db.QueryRow(ctx, query, id).Scan(
		&auction.ID,
		&auction.RideRequestID,
		&amount,
		&auction.Currency,
		&auction.Status,
		&auction.SelectedOfferID,
		&auction.OpenedAt,
		&auction.ExpiresAt,
		&auction.ResolvedAt,
		&auction.CreatedAt,
		&auction.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf(
				"flex auction not found: %w",
				repository.ErrNotFound,
			)
		}
		return nil, fmt.Errorf("get flex auction: %w", err)
	}

	auction.PassengerOfferCents, err = flexDecimalToCents(amount)
	if err != nil {
		return nil, fmt.Errorf("decode flex auction amount: %w", err)
	}

	return auction, nil
}
