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

// FlexOfferRepository implements repository.FlexOfferRepository.
type FlexOfferRepository struct {
	db DBTX
}

var _ repository.FlexOfferRepository = (*FlexOfferRepository)(nil)

func NewFlexOfferRepository(
	db *pgxpool.Pool,
) repository.FlexOfferRepository {
	return &FlexOfferRepository{db: db}
}

func NewFlexOfferRepositoryWithDB(
	db DBTX,
) repository.FlexOfferRepository {
	return &FlexOfferRepository{db: db}
}

// Create persists a new PENDING driver bid.
//
// The database enforces the assignment-bound identity and one-offer-per-driver
// uniqueness. Transactional services enforce auction and driver eligibility.
func (r *FlexOfferRepository) Create(
	ctx context.Context,
	offer *models.FlexOffer,
) error {
	if offer == nil {
		return errors.New("flex offer is required")
	}

	if offer.AuctionID == "" ||
		offer.AssignmentID == "" ||
		offer.DriverID == "" ||
		offer.VehicleID == "" ||
		offer.CompanyID == "" ||
		offer.BranchID == "" ||
		offer.FleetID == "" {
		return errors.New("flex offer identity fields are required")
	}

	// Preserve compatibility with existing acceptance callers.
	// Counter-offer callers must explicitly select COUNTER_OFFER.
	if offer.OfferType == "" {
		offer.OfferType = models.FlexOfferTypeAcceptFare
	}
	if offer.OfferType != models.FlexOfferTypeAcceptFare &&
		offer.OfferType != models.FlexOfferTypeCounterOffer {
		return errors.New("invalid FLEX offer type")
	}

	amount, err := flexCentsToDecimal(offer.OfferedAmountCents)
	if err != nil {
		return fmt.Errorf("invalid flex offer amount: %w", err)
	}

	if offer.ID == "" {
		offer.ID = uuid.NewString()
	}

	now := time.Now().UTC()

	if offer.OfferedAt.IsZero() {
		offer.OfferedAt = now
	}
	if !offer.ExpiresAt.After(offer.OfferedAt) {
		return errors.New("flex offer expiry must follow submission")
	}

	offer.Status = models.FlexOfferStatusPending
	offer.RespondedAt = nil

	if offer.CreatedAt.IsZero() {
		offer.CreatedAt = now
	}
	offer.UpdatedAt = now

	const query = `
		INSERT INTO flex_offers (
			id,
			auction_id,
			assignment_id,
			driver_id,
			vehicle_id,
			company_id,
			branch_id,
			fleet_id,
			offered_amount,
			offer_type,
			status,
			offered_at,
			expires_at,
			responded_at,
			created_at,
			updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9::numeric,
			$10, 'PENDING', $11, $12, NULL, $13, $14
		)
	`

	_, err = r.db.Exec(
		ctx,
		query,
		offer.ID,
		offer.AuctionID,
		offer.AssignmentID,
		offer.DriverID,
		offer.VehicleID,
		offer.CompanyID,
		offer.BranchID,
		offer.FleetID,
		amount,
		offer.OfferType,
		offer.OfferedAt,
		offer.ExpiresAt,
		offer.CreatedAt,
		offer.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create flex offer: %w", err)
	}

	return nil
}

const flexOfferColumns = `
	id,
	auction_id,
	assignment_id,
	driver_id,
	vehicle_id,
	company_id,
	branch_id,
	fleet_id,
	offered_amount::text,
	offer_type,
	status,
	offered_at,
	expires_at,
	responded_at,
	created_at,
	updated_at
`

func (r *FlexOfferRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.FlexOffer, error) {
	return r.getOne(
		ctx,
		"SELECT "+flexOfferColumns+" FROM flex_offers WHERE id = $1",
		id,
	)
}

// GetByIDForUpdate must be called with a transaction-bound repository.
func (r *FlexOfferRepository) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.FlexOffer, error) {
	return r.getOne(
		ctx,
		"SELECT "+flexOfferColumns+
			" FROM flex_offers WHERE id = $1 FOR UPDATE",
		id,
	)
}

func (r *FlexOfferRepository) ListByAuctionID(
	ctx context.Context,
	auctionID string,
) ([]*models.FlexOffer, error) {
	query := "SELECT " + flexOfferColumns +
		" FROM flex_offers WHERE auction_id = $1" +
		" ORDER BY offered_at ASC, id ASC"

	rows, err := r.db.Query(ctx, query, auctionID)
	if err != nil {
		return nil, fmt.Errorf("list flex offers: %w", err)
	}
	defer rows.Close()

	offers := make([]*models.FlexOffer, 0)

	for rows.Next() {
		offer, err := scanFlexOffer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan flex offer: %w", err)
		}
		offers = append(offers, offer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate flex offers: %w", err)
	}

	return offers, nil
}

func (r *FlexOfferRepository) getOne(
	ctx context.Context,
	query string,
	id string,
) (*models.FlexOffer, error) {
	offer, err := scanFlexOffer(r.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf(
				"flex offer not found: %w",
				repository.ErrNotFound,
			)
		}
		return nil, fmt.Errorf("get flex offer: %w", err)
	}

	return offer, nil
}

func scanFlexOffer(row pgx.Row) (*models.FlexOffer, error) {
	offer := &models.FlexOffer{}
	var amount string

	err := row.Scan(
		&offer.ID,
		&offer.AuctionID,
		&offer.AssignmentID,
		&offer.DriverID,
		&offer.VehicleID,
		&offer.CompanyID,
		&offer.BranchID,
		&offer.FleetID,
		&amount,
		&offer.OfferType,
		&offer.Status,
		&offer.OfferedAt,
		&offer.ExpiresAt,
		&offer.RespondedAt,
		&offer.CreatedAt,
		&offer.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	offer.OfferedAmountCents, err = flexDecimalToCents(amount)
	if err != nil {
		return nil, fmt.Errorf("decode flex offer amount: %w", err)
	}

	return offer, nil
}
