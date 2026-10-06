package company

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Delete archives a company.
//
// A company cannot be archived while any non-archived branch belongs to it.
// No branch lifecycle state is changed implicitly.
func (s *Service) Delete(
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
			"resolve company archive authority: %w",
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
					"lock company lifecycle for archive: %w",
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

			hasBranches, err := branches.HasNonDeletedByCompany(
				ctx,
				id,
			)
			if err != nil {
				return fmt.Errorf(
					"check company branches before archive: %w",
					err,
				)
			}
			if hasBranches {
				return ErrCompanyHasBranches
			}

			if systemAdmin {
				return companies.Archive(ctx, id)
			}

			return companies.ArchiveForCompanyMember(
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
