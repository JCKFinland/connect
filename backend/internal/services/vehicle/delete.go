package vehicle

import (
	"context"
	"errors"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var ErrVehicleHasActiveAssignment = errors.New(
	"vehicle has an active driver assignment",
)

// Delete archives a vehicle after enforcing tenant and lifecycle authority.
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

		return ErrVehicleCreationAccessDenied
	}

	systemAdmin, err := s.isSystemAdmin(
		ctx,
		userID,
	)
	if err != nil {
		return fmt.Errorf(
			"resolve vehicle archive authority: %w",
			err,
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			// Assignment creation acquires this same transaction-scoped
			// advisory lock before reading vehicle lifecycle state. Sharing
			// the lock serializes assignment and archival decisions.
			if err := postgresrepo.AcquireTransactionAdvisoryLock(
				ctx,
				tx,
				"vehicle:"+id,
			); err != nil {
				return fmt.Errorf(
					"lock vehicle lifecycle for archive: %w",
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

			// Resolve the target through the caller's authority boundary.
			// Non-system users cannot distinguish a cross-tenant vehicle
			// from a nonexistent vehicle.
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

			// An open assignment is canonical evidence that this vehicle is
			// still operationally owned by a driver and therefore cannot be
			// archived.
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
				// No active assignment. Archival may proceed.

			default:
				return fmt.Errorf(
					"check active vehicle assignment: %w",
					err,
				)
			}

			if systemAdmin {
				return vehicles.Archive(
					ctx,
					id,
				)
			}

			// Repeat the membership predicate in the mutation itself so the
			// repository remains fail-closed even if this service evolves.
			return vehicles.ArchiveForCompanyMember(
				ctx,
				userID,
				id,
			)
		},
	)
}
