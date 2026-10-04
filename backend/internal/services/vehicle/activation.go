package vehicle

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var ErrVehicleFleetInactive = errors.New(
	"vehicle cannot be reactivated while its fleet is inactive",
)

// Deactivate makes a vehicle operationally ineligible after enforcing tenant
// authority and active-assignment lifecycle integrity.
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

		return ErrVehicleCreationAccessDenied
	}

	systemAdmin, err := s.isSystemAdmin(
		ctx,
		userID,
	)
	if err != nil {
		return fmt.Errorf(
			"resolve vehicle deactivation authority: %w",
			err,
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"vehicle:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock vehicle lifecycle for deactivation: %w",
					err,
				)
			}

			vehicles :=
				postgresrepo.NewVehicleRepositoryWithDB(
					tx,
				)

			assignments :=
				postgresrepo.NewDriverAssignmentRepositoryWithDB(
					tx,
				)

			if systemAdmin {
				if _, err := vehicles.GetByID(
					ctx,
					id,
				); err != nil {
					return err
				}
			} else {
				if _, err := vehicles.GetByIDForCompanyMember(
					ctx,
					userID,
					id,
				); err != nil {
					return err
				}
			}

			_, err := assignments.GetActiveByVehicle(
				ctx,
				id,
			)

			switch {
			case err == nil:
				return ErrVehicleHasActiveAssignment

			case errors.Is(
				err,
				repository.ErrNotFound,
			):
				// No active assignment. Deactivation may proceed.

			default:
				return fmt.Errorf(
					"check active vehicle assignment: %w",
					err,
				)
			}

			if systemAdmin {
				return vehicles.Deactivate(
					ctx,
					id,
				)
			}

			return vehicles.DeactivateForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}

// Reactivate restores operational eligibility after enforcing tenant authority
// and the owning fleet's lifecycle state.
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

		return ErrVehicleCreationAccessDenied
	}

	systemAdmin, err := s.isSystemAdmin(
		ctx,
		userID,
	)
	if err != nil {
		return fmt.Errorf(
			"resolve vehicle reactivation authority: %w",
			err,
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"vehicle:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock vehicle lifecycle for reactivation: %w",
					err,
				)
			}

			vehicles :=
				postgresrepo.NewVehicleRepositoryWithDB(
					tx,
				)

			if systemAdmin {
				if _, err := vehicles.GetByID(
					ctx,
					id,
				); err != nil {
					return err
				}
			} else {
				if _, err := vehicles.GetByIDForCompanyMember(
					ctx,
					userID,
					id,
				); err != nil {
					return err
				}
			}

			fleetActive, err :=
				vehicles.IsOwningFleetActive(
					ctx,
					id,
				)
			switch {
			case errors.Is(
				err,
				repository.ErrNotFound,
			):
				return ErrVehicleFleetInactive

			case err != nil:
				return fmt.Errorf(
					"check vehicle fleet lifecycle: %w",
					err,
				)

			case !fleetActive:
				return ErrVehicleFleetInactive
			}

			if systemAdmin {
				return vehicles.Reactivate(
					ctx,
					id,
				)
			}

			return vehicles.ReactivateForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}
