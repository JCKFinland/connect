package company

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Deactivate makes a company operationally inactive.
//
// An active branch implies an active, non-archived owning company, so company
// deactivation is rejected while any active, non-archived branch exists.
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
		return ErrCompanyNotFound
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return fmt.Errorf(
			"resolve company deactivation authority: %w",
			err,
		)
	}

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"company:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock company lifecycle for deactivation: %w",
					err,
				)
			}

			companies := postgresrepo.NewCompanyRepositoryWithDB(tx)
			branches := postgresrepo.NewBranchRepositoryWithDB(tx)

			if systemAdmin {
				if _, err := companies.GetByID(ctx, id); err != nil {
					return err
				}
			} else {
				if _, err := companies.GetByIDForCompanyMember(
					ctx,
					userID,
					id,
				); err != nil {
					return err
				}
			}

			hasActiveBranches, err := branches.HasActiveByCompany(
				ctx,
				id,
			)
			if err != nil {
				return fmt.Errorf(
					"check active company branches before deactivation: %w",
					err,
				)
			}
			if hasActiveBranches {
				return ErrCompanyHasActiveBranches
			}

			if systemAdmin {
				return companies.Deactivate(ctx, id)
			}

			return companies.DeactivateForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)

	if errors.Is(err, repository.ErrNotFound) {
		return ErrCompanyNotFound
	}

	return err
}

// Reactivate makes a non-archived company operationally active.
//
// Company is the lifecycle root, so there is no parent lifecycle dependency.
// Tenant authority still applies.
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
		return ErrCompanyNotFound
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return fmt.Errorf(
			"resolve company reactivation authority: %w",
			err,
		)
	}

	err = postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"company:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock company lifecycle for reactivation: %w",
					err,
				)
			}

			companies := postgresrepo.NewCompanyRepositoryWithDB(tx)

			if systemAdmin {
				if _, err := companies.GetByID(ctx, id); err != nil {
					return err
				}
				return companies.Reactivate(ctx, id)
			}

			if _, err := companies.GetByIDForCompanyMember(
				ctx,
				userID,
				id,
			); err != nil {
				return err
			}

			return companies.ReactivateForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)

	if errors.Is(err, repository.ErrNotFound) {
		return ErrCompanyNotFound
	}

	return err
}
