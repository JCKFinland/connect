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

// DispatchOfferRepository implements repository.DispatchOfferRepository.
type DispatchOfferRepository struct {
	db DBTX
}

var _ repository.DispatchOfferRepository = (*DispatchOfferRepository)(nil)

// NewDispatchOfferRepository creates a repository backed by the connection pool.
func NewDispatchOfferRepository(
	db *pgxpool.Pool,
) repository.DispatchOfferRepository {
	return &DispatchOfferRepository{
		db: db,
	}
}

// NewDispatchOfferRepositoryWithDB creates a repository backed by either
// the connection pool or an active PostgreSQL transaction.
func NewDispatchOfferRepositoryWithDB(
	db DBTX,
) repository.DispatchOfferRepository {
	return &DispatchOfferRepository{
		db: db,
	}
}

// Create inserts a new dispatch offer.
func (r *DispatchOfferRepository) Create(
	ctx context.Context,
	offer *models.DispatchOffer,
) error {

	if offer == nil {
		return fmt.Errorf("dispatch offer is required")
	}

	if offer.ID == "" {
		offer.ID = uuid.NewString()
	}

	const query = `
		INSERT INTO dispatch_offers
		(
			id,
			ride_request_id,
			driver_id,
			vehicle_id,
			company_id,
			branch_id,
			fleet_id,
			status,
			offered_at,
			expires_at,
			responded_at,
			rejection_reason,
			created_by,
			created_at,
			updated_at
		)
		VALUES
		(
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,$10,
			$11,$12,$13,$14,$15
		)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		offer.ID,
		offer.RideRequestID,
		offer.DriverID,
		offer.VehicleID,
		offer.CompanyID,
		offer.BranchID,
		offer.FleetID,
		offer.Status,
		offer.OfferedAt,
		offer.ExpiresAt,
		offer.RespondedAt,
		offer.RejectionReason,
		offer.CreatedBy,
		offer.CreatedAt,
		offer.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf(
			"create dispatch offer: %w",
			err,
		)
	}

	return nil
}

// GetByID retrieves a dispatch offer by ID.
func (r *DispatchOfferRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.DispatchOffer, error) {

	const query = `
		SELECT
			id,
			ride_request_id,
			driver_id,
			vehicle_id,
			company_id,
			branch_id,
			fleet_id,
			status,
			offered_at,
			expires_at,
			responded_at,
			rejection_reason,
			created_by,
			created_at,
			updated_at
		FROM dispatch_offers
		WHERE id = $1
	`

	return r.getOne(
		ctx,
		query,
		id,
	)
}

// GetPendingByDriver retrieves the driver's active PENDING dispatch offer.
func (r *DispatchOfferRepository) GetPendingByDriver(
	ctx context.Context,
	driverID string,
) (*models.DispatchOffer, error) {

	const query = `
		SELECT
			id,
			ride_request_id,
			driver_id,
			vehicle_id,
			company_id,
			branch_id,
			fleet_id,
			status,
			offered_at,
			expires_at,
			responded_at,
			rejection_reason,
			created_by,
			created_at,
			updated_at
		FROM dispatch_offers
		WHERE driver_id = $1
		  AND status = 'PENDING'
		LIMIT 1
	`

	return r.getOne(
		ctx,
		query,
		driverID,
	)
}

// GetPendingByRideRequest retrieves the active PENDING offer for a ride request.
func (r *DispatchOfferRepository) GetPendingByRideRequest(
	ctx context.Context,
	rideRequestID string,
) (*models.DispatchOffer, error) {

	const query = `
		SELECT
			id,
			ride_request_id,
			driver_id,
			vehicle_id,
			company_id,
			branch_id,
			fleet_id,
			status,
			offered_at,
			expires_at,
			responded_at,
			rejection_reason,
			created_by,
			created_at,
			updated_at
		FROM dispatch_offers
		WHERE ride_request_id = $1
		  AND status = 'PENDING'
		LIMIT 1
	`

	return r.getOne(
		ctx,
		query,
		rideRequestID,
	)
}

// getOne executes a dispatch-offer query returning a single row.
func (r *DispatchOfferRepository) getOne(
	ctx context.Context,
	query string,
	arg any,
) (*models.DispatchOffer, error) {

	offer := &models.DispatchOffer{}

	err := r.db.QueryRow(
		ctx,
		query,
		arg,
	).Scan(
		&offer.ID,
		&offer.RideRequestID,
		&offer.DriverID,
		&offer.VehicleID,
		&offer.CompanyID,
		&offer.BranchID,
		&offer.FleetID,
		&offer.Status,
		&offer.OfferedAt,
		&offer.ExpiresAt,
		&offer.RespondedAt,
		&offer.RejectionReason,
		&offer.CreatedBy,
		&offer.CreatedAt,
		&offer.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"get dispatch offer: %w",
			err,
		)
	}

	return offer, nil
}

// GetByIDForUpdate retrieves and locks a dispatch offer row.
//
// This method must be called inside an active PostgreSQL transaction.
// The row remains locked until that transaction commits or rolls back.
func (r *DispatchOfferRepository) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.DispatchOffer, error) {

	const query = `
		SELECT
			id,
			ride_request_id,
			driver_id,
			vehicle_id,
			company_id,
			branch_id,
			fleet_id,
			status,
			offered_at,
			expires_at,
			responded_at,
			rejection_reason,
			created_by,
			created_at,
			updated_at
		FROM dispatch_offers
		WHERE id = $1
		FOR UPDATE
	`

	return r.getOne(
		ctx,
		query,
		id,
	)
}

// UpdateStatus performs a lifecycle update for a dispatch offer.
func (r *DispatchOfferRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
	respondedAt *time.Time,
	rejectionReason *string,
) error {

	const query = `
		UPDATE dispatch_offers
		SET
			status = $2,
			responded_at = $3,
			rejection_reason = $4,
			updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.db.Exec(
		ctx,
		query,
		id,
		status,
		respondedAt,
		rejectionReason,
	)
	if err != nil {
		return fmt.Errorf(
			"update dispatch offer status: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// ExpireStalePending marks all time-expired PENDING offers as EXPIRED.
//
// It returns the ride-request IDs associated with the offers that were
// expired. The caller can then reset MATCHING ride requests back to
// PENDING inside the same PostgreSQL transaction.
func (r *DispatchOfferRepository) ExpireStalePending(
	ctx context.Context,
	now time.Time,
) ([]string, error) {

	const query = `
		UPDATE dispatch_offers
		SET
			status = 'EXPIRED',
			responded_at = $1,
			updated_at = $1
		WHERE status = 'PENDING'
		  AND expires_at <= $1
		RETURNING ride_request_id
	`

	rows, err := r.db.Query(
		ctx,
		query,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"expire stale dispatch offers: %w",
			err,
		)
	}
	defer rows.Close()

	rideRequestIDs := make([]string, 0)

	for rows.Next() {
		var rideRequestID string

		if err := rows.Scan(
			&rideRequestID,
		); err != nil {
			return nil, fmt.Errorf(
				"scan expired dispatch offer ride request: %w",
				err,
			)
		}

		rideRequestIDs = append(
			rideRequestIDs,
			rideRequestID,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate expired dispatch offers: %w",
			err,
		)
	}

	return rideRequestIDs, nil
}
