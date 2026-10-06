package branch

import (
	"context"
	"fmt"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Deactivate makes a branch operationally inactive.
//
// An active fleet implies an active, non-archived owning branch, so branch
// deactivation is rejected while any active, non-archived fleet exists.
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
		return fmt.Errorf("branch deactivation access denied")
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return fmt.Errorf(
			"resolve branch deactivation authority: %w",
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
				"branch:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock branch lifecycle for deactivation: %w",
					err,
				)
			}

			branches := postgresrepo.NewBranchRepositoryWithDB(tx)
			fleets := postgresrepo.NewFleetRepositoryWithDB(tx)

			if systemAdmin {
				if _, err := branches.GetByID(ctx, id); err != nil {
					return err
				}
			} else {
				if _, err := branches.GetByIDForCompanyMember(
					ctx,
					userID,
					id,
				); err != nil {
					return err
				}
			}

			hasActiveFleets, err := fleets.HasActiveByBranch(
				ctx,
				id,
			)
			if err != nil {
				return fmt.Errorf(
					"check active branch fleets before deactivation: %w",
					err,
				)
			}
			if hasActiveFleets {
				return ErrBranchHasActiveFleets
			}

			if systemAdmin {
				return branches.Deactivate(ctx, id)
			}

			return branches.DeactivateForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}

// Reactivate makes a non-archived branch operationally active.
//
// Branch has no lifecycle parent above it, but tenant authority still applies.
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
		return fmt.Errorf("branch reactivation access denied")
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return fmt.Errorf(
			"resolve branch reactivation authority: %w",
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
				"branch:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock branch lifecycle for reactivation: %w",
					err,
				)
			}

			branches := postgresrepo.NewBranchRepositoryWithDB(tx)

			if systemAdmin {
				if _, err := branches.GetByID(ctx, id); err != nil {
					return err
				}
				return branches.Reactivate(ctx, id)
			}

			if _, err := branches.GetByIDForCompanyMember(
				ctx,
				userID,
				id,
			); err != nil {
				return err
			}

			return branches.ReactivateForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}
