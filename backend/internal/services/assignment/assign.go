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
	userID string,
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

	if userID == "" {
		return nil, errors.New(
			"authenticated user ID is required",
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

			drivers :=
				postgresrepo.NewDriverRepositoryWithDB(
					tx,
				)

			vehicles :=
				postgresrepo.NewVehicleRepositoryWithDB(
					tx,
				)

			companies :=
				postgresrepo.NewCompanyRepositoryWithDB(tx)

			branches :=
				postgresrepo.NewBranchRepositoryWithDB(tx)

			fleets :=
				postgresrepo.NewFleetRepositoryWithDB(tx)

			// ---------------------------------------------------------
			// 1. Establish deterministic lifecycle lock targets.
			//
			// These reads establish lock targets only. Driver and vehicle
			// state are authoritatively re-read after all lifecycle locks
			// are held.
			// ---------------------------------------------------------

			preDriver, err := drivers.GetByUserID(ctx, userID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotFound
			}
			if err != nil {
				return fmt.Errorf("pre-read authenticated driver: %w", err)
			}

			fleetID, err := vehicles.GetOwningFleetID(ctx, req.VehicleID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrVehicleNotFound
			}
			if err != nil {
				return fmt.Errorf("pre-read vehicle owning fleet: %w", err)
			}

			lockTargets := []struct {
				key  string
				name string
			}{
				{"company:" + preDriver.CompanyID, "company"},
				{"branch:" + preDriver.BranchID, "branch"},
				{"fleet:" + fleetID, "fleet"},
				{"vehicle:" + req.VehicleID, "vehicle"},
			}

			for _, target := range lockTargets {
				if err := postgresrepo.AcquireTransactionAdvisoryLock(
					ctx,
					tx,
					target.key,
				); err != nil {
					return fmt.Errorf(
						"lock %s lifecycle for assignment: %w",
						target.name,
						err,
					)
				}
			}

			// ---------------------------------------------------------
			// 2. Re-read authoritative driver and vehicle state.
			// ---------------------------------------------------------

			driver, err := drivers.GetByUserIDForUpdate(ctx, userID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotFound
			}
			if err != nil {
				return fmt.Errorf("get authenticated driver: %w", err)
			}

			if driver.CompanyID != preDriver.CompanyID ||
				driver.BranchID != preDriver.BranchID {
				return fmt.Errorf(
					"driver organizational scope changed during assignment",
				)
			}

			if !driver.IsActive ||
				!driver.IsVerified ||
				driver.Status != "ACTIVE" {
				return ErrDriverInactive
			}

			vehicle, err := vehicles.GetByID(ctx, req.VehicleID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrVehicleNotFound
			}
			if err != nil {
				return fmt.Errorf("get assignment vehicle: %w", err)
			}

			if vehicle.FleetID != fleetID {
				return fmt.Errorf(
					"vehicle owning fleet changed during assignment",
				)
			}

			if vehicle.CompanyID != driver.CompanyID ||
				vehicle.BranchID != driver.BranchID {
				return ErrVehicleOutsideDriverScope
			}

			if !vehicle.IsActive {
				return ErrVehicleInactive
			}

			company, err := companies.GetByID(ctx, driver.CompanyID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrCompanyInactive
			}
			if err != nil {
				return fmt.Errorf("get driver company: %w", err)
			}
			if !company.IsActive {
				return ErrCompanyInactive
			}

			branch, err := branches.GetByID(ctx, driver.BranchID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrBranchInactive
			}
			if err != nil {
				return fmt.Errorf("get driver branch: %w", err)
			}
			if branch.CompanyID != driver.CompanyID {
				return ErrVehicleOutsideDriverScope
			}
			if !branch.IsActive {
				return ErrBranchInactive
			}

			fleet, err := fleets.GetByID(ctx, fleetID)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrFleetInactive
			}
			if err != nil {
				return fmt.Errorf("get vehicle fleet: %w", err)
			}
			if fleet.CompanyID != driver.CompanyID ||
				fleet.BranchID != driver.BranchID {
				return ErrVehicleOutsideDriverScope
			}
			if !fleet.IsActive {
				return ErrFleetInactive
			}

			// ---------------------------------------------------------
			// 2. Lock driver lifecycle state.
			//
			// AcceptOffer() and Unassign() use the same presence row
			// as their driver-level serialization point.
			// ---------------------------------------------------------

			_, err =
				presenceRepo.GetByDriverIDForUpdate(
					ctx,
					userID,
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
			// 3. Driver must not already have an active assignment.
			// ---------------------------------------------------------

			_, err =
				assignments.GetActiveByDriver(
					ctx,
					userID,
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
			// 4. Vehicle must not already have an active assignment.
			// ---------------------------------------------------------

			_, err =
				assignments.GetActiveByVehicle(
					ctx,
					vehicle.ID,
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
			// 5. Create assignment from authoritative relationships.
			//
			// PostgreSQL partial unique indexes remain the final
			// concurrency backstop for driver and vehicle uniqueness.
			// ---------------------------------------------------------

			assignment := &models.DriverAssignment{
				CompanyID: driver.CompanyID,
				BranchID:  driver.BranchID,
				FleetID:   vehicle.FleetID,

				DriverID:  userID,
				VehicleID: vehicle.ID,

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
			// 6. Attach assignment to presence only if the driver is
			//    still operationally idle.
			//
			// Failure here rolls back assignment creation as well.
			// ---------------------------------------------------------

			attached, err :=
				presenceRepo.AttachAssignmentIfIdle(
					ctx,
					userID,
					driver.CompanyID,
					driver.BranchID,
					vehicle.ID,
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
