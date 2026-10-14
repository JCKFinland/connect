package flex_offer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	drivereligibility "github.com/JCKFinland/connect/backend/internal/services/driver_eligibility"
	vehicleeligibility "github.com/JCKFinland/connect/backend/internal/services/vehicle_eligibility"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SubmitAcceptFare submits one driver offer at the passenger's
// immutable auction fare.
//
// The transaction does not create a trip, authorize payment,
// or change driver availability.
func (s *Service) SubmitAcceptFare(
	ctx context.Context,
	auctionID string,
	driverUserID string,
) (*models.FlexOffer, error) {
	if s == nil || s.db == nil || s.cfg == nil {
		return nil, errors.New("FLEX offer service is not configured")
	}

	if auctionID == "" || driverUserID == "" {
		return nil, ErrAccessDenied
	}

	// Resolve the ride-request identity before acquiring locks.
	// All authoritative values are revalidated inside the transaction.
	initial, err := postgresrepo.NewFlexAuctionRepository(s.db).
		GetByID(ctx, auctionID)
	if err != nil {
		return nil, fmt.Errorf("resolve FLEX auction: %w", err)
	}

	if initial == nil || initial.RideRequestID == "" {
		return nil, ErrAuctionNotEligible
	}

	var created *models.FlexOffer

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"ride-dispatch:"+initial.RideRequestID,
			); err != nil {
				return fmt.Errorf("lock FLEX ride request: %w", err)
			}

			requests := postgresrepo.NewRideRequestRepositoryWithDB(tx)
			request, err := requests.GetByIDForUpdate(
				ctx,
				initial.RideRequestID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrAuctionNotEligible
			}
			if err != nil {
				return fmt.Errorf("lock FLEX ride request: %w", err)
			}

			auctions := postgresrepo.NewFlexAuctionRepositoryWithDB(tx)
			auction, err := auctions.GetByIDForUpdate(ctx, auctionID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrAuctionNotEligible
			}
			if err != nil {
				return fmt.Errorf("lock FLEX auction: %w", err)
			}

			now := time.Now().UTC()

			if request == nil || auction == nil ||
				auction.RideRequestID != request.ID ||
				request.MatchingMode != models.RideMatchingModeFlex ||
				request.Status != "PENDING" ||
				auction.Status != models.FlexAuctionStatusOpen ||
				auction.SelectedOfferID != nil ||
				!now.Before(auction.ExpiresAt.UTC()) ||
				auction.Currency != "EUR" ||
				auction.PassengerOfferCents <= 0 {
				return ErrAuctionNotEligible
			}

			if request.ExpiresAt != nil &&
				!now.Before(request.ExpiresAt.UTC()) {
				return ErrAuctionNotEligible
			}

			roles, err := repository.NewUserRoleRepository(tx).
				GetUserRoles(ctx, driverUserID)
			if err != nil {
				return fmt.Errorf("authorize FLEX driver: %w", err)
			}

			isDriver := false
			for _, role := range roles {
				if role == "DRIVER" {
					isDriver = true
					break
				}
			}
			if !isDriver {
				return ErrAccessDenied
			}

			// Capture the assignment identity before lifecycle validation.
			// The validator must confirm that this exact assignment
			// remains authoritative while hierarchy locks are acquired.
			assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)
			initialAssignment, err := assignments.GetActiveByDriver(
				ctx,
				driverUserID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotEligible
			}
			if err != nil {
				return fmt.Errorf("resolve initial FLEX assignment: %w", err)
			}
			if !drivereligibility.ValidAssignment(
				initialAssignment,
				driverUserID,
				nil,
			) {
				return ErrDriverNotEligible
			}

			expected := &drivereligibility.LifecycleExpectation{
				AssignmentID: initialAssignment.ID,
				CompanyID:    initialAssignment.CompanyID,
				BranchID:     initialAssignment.BranchID,
				FleetID:      initialAssignment.FleetID,
				VehicleID:    initialAssignment.VehicleID,
			}

			eligible, err := drivereligibility.LockAndValidateLifecycle(
				ctx,
				tx,
				driverUserID,
				expected,
				now,
			)
			if err != nil {
				return fmt.Errorf("validate FLEX driver lifecycle: %w", err)
			}
			if !eligible {
				return ErrDriverNotEligible
			}

			presenceRepo := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)
			presence, err := presenceRepo.GetAvailableByDriverIDForUpdate(
				ctx,
				driverUserID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotEligible
			}
			if err != nil {
				return fmt.Errorf("lock FLEX driver presence: %w", err)
			}

			if presence == nil || presence.LastHeartbeatAt == nil ||
				s.cfg.Presence.HeartbeatTimeout <= 0 {
				return ErrDriverNotEligible
			}

			// Recheck heartbeat freshness after lifecycle and presence locks.
			presenceCheckedAt := time.Now().UTC()
			age := presenceCheckedAt.Sub(presence.LastHeartbeatAt.UTC())
			if age < 0 || age > s.cfg.Presence.HeartbeatTimeout {
				return ErrDriverNotEligible
			}

			assignment, err := assignments.GetActiveByDriver(
				ctx,
				driverUserID,
			)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotEligible
			}
			if err != nil {
				return fmt.Errorf("resolve FLEX driver assignment: %w", err)
			}

			if !drivereligibility.ValidAssignment(
				assignment,
				driverUserID,
				nil,
			) || assignment.UnassignedAt != nil ||
				!drivereligibility.SameAssignment(
					initialAssignment,
					assignment,
				) {
				return ErrDriverNotEligible
			}

			if presence.DriverID != driverUserID ||
				presence.AssignmentID == nil ||
				*presence.AssignmentID != assignment.ID ||
				presence.VehicleID == nil ||
				*presence.VehicleID != assignment.VehicleID ||
				presence.CompanyID != assignment.CompanyID ||
				presence.BranchID == nil ||
				*presence.BranchID != assignment.BranchID {
				return ErrDriverNotEligible
			}

			vehicle, err := postgresrepo.NewVehicleRepositoryWithDB(tx).
				GetByID(ctx, assignment.VehicleID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotEligible
			}
			if err != nil {
				return fmt.Errorf("resolve FLEX vehicle: %w", err)
			}

			if vehicle == nil || !vehicle.IsActive ||
				!vehicleeligibility.IsEligible(
					request.RequestedVehicleType,
					vehicle.VehicleType,
				) {
				return ErrDriverNotEligible
			}

			// Capture a fresh timestamp after potentially blocking locks.
			submittedAt := time.Now().UTC()
			if !submittedAt.Before(auction.ExpiresAt.UTC()) {
				return ErrAuctionNotEligible
			}
			if request.ExpiresAt != nil &&
				!submittedAt.Before(request.ExpiresAt.UTC()) {
				return ErrAuctionNotEligible
			}

			offer := &models.FlexOffer{
				AuctionID:          auction.ID,
				AssignmentID:       assignment.ID,
				DriverID:           driverUserID,
				VehicleID:          assignment.VehicleID,
				CompanyID:          assignment.CompanyID,
				BranchID:           assignment.BranchID,
				FleetID:            assignment.FleetID,
				OfferedAmountCents: auction.PassengerOfferCents,
				OfferedAt:          submittedAt,
				ExpiresAt:          auction.ExpiresAt.UTC(),
			}

			if err := postgresrepo.NewFlexOfferRepositoryWithDB(tx).
				Create(ctx, offer); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) &&
					pgErr.Code == "23505" &&
					pgErr.ConstraintName == "uq_flex_offers_auction_driver" {
					return ErrOfferAlreadyExists
				}
				return fmt.Errorf("persist FLEX driver offer: %w", err)
			}

			// PostgreSQL clock_timestamp() reflects actual wall-clock time,
			// including time spent waiting on locks or INSERT execution.
			// Rejecting here rolls back the inserted offer atomically.
			var stillOpen bool
			if err := tx.QueryRow(ctx, `
				SELECT
					clock_timestamp() < expires_at
					AND status = 'OPEN'
					AND selected_offer_id IS NULL
				FROM flex_auctions
				WHERE id = $1
			`, auction.ID).Scan(&stillOpen); err != nil {
				return fmt.Errorf("recheck FLEX auction expiry: %w", err)
			}
			if !stillOpen {
				return ErrAuctionNotEligible
			}

			if request.ExpiresAt != nil {
				var requestStillOpen bool
				if err := tx.QueryRow(ctx, `
					SELECT
						clock_timestamp() < expires_at
						AND status = 'PENDING'
					FROM ride_requests
					WHERE id = $1
				`, request.ID).Scan(&requestStillOpen); err != nil {
					return fmt.Errorf("recheck FLEX ride request expiry: %w", err)
				}
				if !requestStillOpen {
					return ErrAuctionNotEligible
				}
			}

			created = offer
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return created, nil
}
