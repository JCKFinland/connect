package assignment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/presence"
	"github.com/jackc/pgx/v5"
)

func (s *Service) Assign(
	ctx context.Context,
	req AssignDriverRequest,
) (*models.DriverAssignment, error) {

	if s == nil {
		return nil, errors.New(
			"assignment service is required",
		)
	}

	if s.db == nil {
		return nil, errors.New(
			"assignment database is not configured",
		)
	}

	if req.DriverID == "" {
		return nil, errors.New(
			"driver ID is required",
		)
	}

	if req.VehicleID == "" {
		return nil, errors.New(
			"vehicle ID is required",
		)
	}

	var createdAssignment *models.DriverAssignment

	err := postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			assignments :=
				postgresrepo.NewDriverAssignmentRepositoryWithDB(
					tx,
				)

			presenceRepo :=
				postgresrepo.NewDriverPresenceRepositoryWithDB(
					tx,
				)

			// ---------------------------------------------------------
			// 1. Lock driver lifecycle state.
			//
			// AcceptOffer() and Unassign() use the same presence row
			// as their driver-level serialization point.
			// ---------------------------------------------------------

			_, err :=
				presenceRepo.GetByDriverIDForUpdate(
					ctx,
					req.DriverID,
				)

			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return fmt.Errorf(
					"driver presence is required before assignment",
				)
			}

			if err != nil {
				return fmt.Errorf(
					"lock driver presence for assignment: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 2. Driver must not already have an active assignment.
			// ---------------------------------------------------------

			_, err =
				assignments.GetActiveByDriver(
					ctx,
					req.DriverID,
				)

			if err == nil {
				return ErrDriverAlreadyAssigned
			}

			if !errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return fmt.Errorf(
					"check active driver assignment: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 3. Vehicle must not already have an active assignment.
			// ---------------------------------------------------------

			_, err =
				assignments.GetActiveByVehicle(
					ctx,
					req.VehicleID,
				)

			if err == nil {
				return ErrVehicleAlreadyAssigned
			}

			if !errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return fmt.Errorf(
					"check active vehicle assignment: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 4. Create assignment.
			//
			// PostgreSQL partial unique indexes remain the final
			// concurrency backstop for driver and vehicle uniqueness.
			// ---------------------------------------------------------

			assignment := &models.DriverAssignment{
				CompanyID: req.CompanyID,
				BranchID:  req.BranchID,
				FleetID:   req.FleetID,

				DriverID:  req.DriverID,
				VehicleID: req.VehicleID,

				AssignedAt: time.Now().UTC(),
				Notes:      req.Notes,
			}

			if err := assignments.Create(
				ctx,
				assignment,
			); err != nil {
				return fmt.Errorf(
					"create driver assignment: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// 5. Attach assignment to presence only if the driver is
			//    still operationally idle.
			//
			// Failure here rolls back assignment creation as well.
			// ---------------------------------------------------------

			attached, err :=
				presenceRepo.AttachAssignmentIfIdle(
					ctx,
					req.DriverID,
					req.CompanyID,
					req.BranchID,
					req.VehicleID,
					assignment.ID,
				)

			if err != nil {
				return fmt.Errorf(
					"attach driver assignment presence: %w",
					err,
				)
			}

			if !attached {
				return presence.ErrDriverAvailabilityLocked
			}

			createdAssignment = assignment

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	if createdAssignment == nil {
		return nil, errors.New(
			"driver assignment completed without creating an assignment",
		)
	}

	return createdAssignment, nil
}
