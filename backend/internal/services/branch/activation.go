package branch

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"

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
		s.branches == nil ||
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

	// Pre-read only to establish deterministic company -> branch lock ordering.
	// The branch is authoritatively re-read after both locks are held.
	companyID, err := s.branches.GetOwningCompanyID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return repository.ErrNotFound
		}
		return fmt.Errorf("get branch owning company: %w", err)
	}
	if companyID == "" {
		return repository.ErrNotFound
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"company:"+companyID,
			); err != nil {
				return fmt.Errorf(
					"lock owning company for branch reactivation: %w",
					err,
				)
			}

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

			companies := postgresrepo.NewCompanyRepositoryWithDB(tx)
			branches := postgresrepo.NewBranchRepositoryWithDB(tx)

			var branchCompanyID string

			if systemAdmin {
				branchObj, err := branches.GetByID(ctx, id)
				if err != nil {
					return err
				}
				branchCompanyID = branchObj.CompanyID
			} else {
				branchObj, err := branches.GetByIDForCompanyMember(
					ctx,
					userID,
					id,
				)
				if err != nil {
					return err
				}
				branchCompanyID = branchObj.CompanyID
			}

			// Ownership changing between the pre-read and locked authoritative
			// read invalidates the lock target. Fail closed rather than acting
			// while holding the wrong parent lock.
			if branchCompanyID != companyID {
				return fmt.Errorf(
					"branch owning company changed during reactivation",
				)
			}

			var company *models.Company

			if systemAdmin {
				company, err = companies.GetByID(
					ctx,
					companyID,
				)
			} else {
				company, err = companies.GetByIDForCompanyMember(
					ctx,
					userID,
					companyID,
				)
			}

			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					return ErrInvalidCompany
				}
				return fmt.Errorf(
					"check branch company lifecycle: %w",
					err,
				)
			}

			if company == nil ||
				company.ID == "" ||
				!company.IsActive {
				return ErrInvalidCompany
			}

			if systemAdmin {
				return branches.Reactivate(ctx, id)
			}

			return branches.ReactivateForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}
