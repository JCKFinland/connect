package fleet

import (
	"context"
	"fmt"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Delete archives a fleet after enforcing tenant and lifecycle authority.
//
// Archival is intentionally distinct from operational deactivation:
// this operation does not mutate is_active.
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

		return fmt.Errorf("fleet archive access denied")
	}

	systemAdmin, err := s.isSystemAdmin(
		ctx,
		userID,
	)
	if err != nil {
		return fmt.Errorf(
			"resolve fleet archive authority: %w",
			err,
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			// Vehicle creation and fleet archive share this logical lock so
			// parent/child existence decisions serialize.
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"fleet:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock fleet lifecycle for archive: %w",
					err,
				)
			}

			fleets := postgresrepo.NewFleetRepositoryWithDB(tx)
			vehicles := postgresrepo.NewVehicleRepositoryWithDB(tx)

			// Resolve the target through the caller's authority boundary.
			// Non-system users cannot distinguish a cross-tenant fleet from a
			// nonexistent fleet.
			if systemAdmin {
				if _, err := fleets.GetByID(
					ctx,
					id,
				); err != nil {
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

			hasVehicles, err := vehicles.HasNonDeletedByFleet(
				ctx,
				id,
			)
			if err != nil {
				return fmt.Errorf(
					"check fleet vehicles before archive: %w",
					err,
				)
			}
			if hasVehicles {
				return ErrFleetHasVehicles
			}

			if systemAdmin {
				return fleets.Archive(
					ctx,
					id,
				)
			}

			// Repeat the membership predicate in the mutation itself so the
			// repository remains fail-closed if this service evolves.
			return fleets.ArchiveForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}
