package fleet

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Deactivate makes a fleet operationally inactive after enforcing tenant
// authority and active-vehicle lifecycle integrity.
func (s *Service) Deactivate(
	ctx context.Context,
	userID string,
	id string,
) error {
	if s == nil ||
		s.db == nil ||
		s.userRoles == nil ||
		userID == "" ||
		id == "" {
		return fmt.Errorf("fleet deactivation access denied")
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return fmt.Errorf("resolve fleet deactivation authority: %w", err)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"fleet:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock fleet lifecycle for deactivation: %w",
					err,
				)
			}

			fleets := postgresrepo.NewFleetRepositoryWithDB(tx)
			vehicles := postgresrepo.NewVehicleRepositoryWithDB(tx)

			if systemAdmin {
				if _, err := fleets.GetByID(ctx, id); err != nil {
					return err
				}
			} else {
				if _, err := fleets.GetByIDForCompanyMember(
					ctx,
					userID,
					id,
				); err != nil {
					return err
				}
			}

			hasActive, err := vehicles.HasActiveByFleet(ctx, id)
			if err != nil {
				return fmt.Errorf(
					"check active fleet vehicles: %w",
					err,
				)
			}
			if hasActive {
				return ErrFleetHasActiveVehicles
			}

			if systemAdmin {
				return fleets.Deactivate(ctx, id)
			}

			return fleets.DeactivateForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}

// Reactivate restores fleet operational eligibility after enforcing tenant
// authority and the owning branch's lifecycle state.
func (s *Service) Reactivate(
	ctx context.Context,
	userID string,
	id string,
) error {
	if s == nil ||
		s.db == nil ||
		s.userRoles == nil ||
		userID == "" ||
		id == "" {
		return fmt.Errorf("fleet reactivation access denied")
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return fmt.Errorf("resolve fleet reactivation authority: %w", err)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"fleet:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock fleet lifecycle for reactivation: %w",
					err,
				)
			}

			fleets := postgresrepo.NewFleetRepositoryWithDB(tx)

			if systemAdmin {
				if _, err := fleets.GetByID(ctx, id); err != nil {
					return err
				}
			} else {
				if _, err := fleets.GetByIDForCompanyMember(
					ctx,
					userID,
					id,
				); err != nil {
					return err
				}
			}

			branchActive, err := fleets.IsOwningBranchActive(ctx, id)
			switch {
			case errors.Is(err, repository.ErrNotFound):
				return ErrFleetBranchInactive
			case err != nil:
				return fmt.Errorf(
					"check fleet branch lifecycle: %w",
					err,
				)
			case !branchActive:
				return ErrFleetBranchInactive
			}

			if systemAdmin {
				return fleets.Reactivate(ctx, id)
			}

			return fleets.ReactivateForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}
