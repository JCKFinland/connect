package branch

import (
	"context"
	"fmt"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Delete archives a branch after enforcing tenant and lifecycle authority.
//
// Archival is distinct from operational deactivation and does not mutate
// is_active. A branch cannot be archived while it owns any non-archived fleet.
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
		return fmt.Errorf("branch archive access denied")
	}

	systemAdmin, err := s.isSystemAdmin(ctx, userID)
	if err != nil {
		return fmt.Errorf(
			"resolve branch archive authority: %w",
			err,
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {
			// Fleet creation and branch archive must share this logical lock so
			// parent/child existence decisions serialize.
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"branch:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock branch lifecycle for archive: %w",
					err,
				)
			}

			branches := postgresrepo.NewBranchRepositoryWithDB(tx)
			fleets := postgresrepo.NewFleetRepositoryWithDB(tx)

			if systemAdmin {
				if _, err := branches.GetByID(
					ctx,
					id,
				); err != nil {
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

			hasFleets, err := fleets.HasNonDeletedByBranch(
				ctx,
				id,
			)
			if err != nil {
				return fmt.Errorf(
					"check branch fleets before archive: %w",
					err,
				)
			}
			if hasFleets {
				return ErrBranchHasFleets
			}

			if systemAdmin {
				return branches.Archive(ctx, id)
			}

			return branches.ArchiveForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}
