package presence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var ErrDriverAssignmentRequired = errors.New(
	"driver assignment required before going online",
)

var ErrDriverComplianceRequired = errors.New(
	"driver regulatory compliance required",
)

var (
	ErrDriverInactive         = errors.New("driver inactive")
	ErrCompanyInactive        = errors.New("company inactive")
	ErrBranchInactive         = errors.New("branch inactive")
	ErrFleetInactive          = errors.New("fleet inactive")
	ErrVehicleInactive        = errors.New("vehicle inactive")
	ErrAssignmentScopeInvalid = errors.New("driver assignment scope invalid")
)

func (s *Service) GoOnline(
	ctx context.Context,
	req GoOnlineRequest,
) error {

	if s == nil {
		return errors.New(
			"presence service is required",
		)
	}

	if s.db == nil {
		return errors.New(
			"presence database is not configured",
		)
	}

	if req.UserID == "" {
		return errors.New(
			"user ID is required",
		)
	}

	if s.drivers == nil {
		return errors.New(
			"driver repository is not configured",
		)
	}

	if s.compliance == nil {
		return errors.New(
			"driver compliance service is not configured",
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			presenceRepo :=
				postgresrepo.NewDriverPresenceRepositoryWithDB(
					tx,
				)

			assignmentRepo :=
				postgresrepo.NewDriverAssignmentRepositoryWithDB(
					tx,
				)

			driverRepo :=
				postgresrepo.NewDriverRepositoryWithDB(
					tx,
				)

			companyRepo :=
				postgresrepo.NewCompanyRepositoryWithDB(
					tx,
				)

			branchRepo :=
				postgresrepo.NewBranchRepositoryWithDB(
					tx,
				)

			fleetRepo :=
				postgresrepo.NewFleetRepositoryWithDB(
					tx,
				)

			vehicleRepo :=
				postgresrepo.NewVehicleRepositoryWithDB(
					tx,
				)

			// Establish the complete lifecycle target before taking row locks.
			// The assignment is re-read after presence serialization before
			// publishing the online state.
			preAssignment, err :=
				assignmentRepo.GetActiveByDriver(
					ctx,
					req.UserID,
				)

			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return ErrDriverAssignmentRequired
			}

			if err != nil {
				return fmt.Errorf(
					"get active driver assignment before lifecycle validation: %w",
					err,
				)
			}

			if preAssignment == nil ||
				preAssignment.ID == "" ||
				preAssignment.CompanyID == "" ||
				preAssignment.BranchID == "" ||
				preAssignment.FleetID == "" ||
				preAssignment.VehicleID == "" {

				return ErrDriverAssignmentRequired
			}

			// Lifecycle-enabling reads serialize with parent deactivation and
			// archive operations in deterministic parent-to-child order.
			lifecycleTargets := []struct {
				key  string
				name string
			}{
				{"company:" + preAssignment.CompanyID, "company"},
				{"branch:" + preAssignment.BranchID, "branch"},
				{"fleet:" + preAssignment.FleetID, "fleet"},
				{"vehicle:" + preAssignment.VehicleID, "vehicle"},
			}

			for _, target := range lifecycleTargets {
				if err := postgresrepo.AcquireTransactionAdvisorySharedLock(
					ctx,
					tx,
					target.key,
				); err != nil {
					return fmt.Errorf(
						"acquire %s lifecycle lock before going online: %w",
						target.name,
						err,
					)
				}
			}

			// Lock and re-read the authoritative driver before operational
			// presence. Driver lifecycle operations use this same ordering.
			authoritativeDriver, err :=
				driverRepo.GetByUserIDForUpdate(
					ctx,
					req.UserID,
				)

			if errors.Is(err, repository.ErrNotFound) {
				return ErrDriverNotFound
			}
			if err != nil {
				return fmt.Errorf(
					"get authoritative driver before going online: %w",
					err,
				)
			}

			if authoritativeDriver.CompanyID != preAssignment.CompanyID ||
				authoritativeDriver.BranchID != preAssignment.BranchID {
				return ErrAssignmentScopeInvalid
			}

			if !authoritativeDriver.IsActive ||
				!authoritativeDriver.IsVerified ||
				authoritativeDriver.Status != "ACTIVE" {
				return ErrDriverInactive
			}

			company, err :=
				companyRepo.GetByID(
					ctx,
					preAssignment.CompanyID,
				)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrCompanyInactive
			}
			if err != nil {
				return fmt.Errorf(
					"get company before going online: %w",
					err,
				)
			}
			if company == nil || !company.IsActive {
				return ErrCompanyInactive
			}

			branch, err :=
				branchRepo.GetByID(
					ctx,
					preAssignment.BranchID,
				)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrBranchInactive
			}
			if err != nil {
				return fmt.Errorf(
					"get branch before going online: %w",
					err,
				)
			}
			if branch == nil {
				return ErrBranchInactive
			}
			if branch.CompanyID != preAssignment.CompanyID {
				return ErrAssignmentScopeInvalid
			}
			if !branch.IsActive {
				return ErrBranchInactive
			}

			fleet, err :=
				fleetRepo.GetByID(
					ctx,
					preAssignment.FleetID,
				)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrFleetInactive
			}
			if err != nil {
				return fmt.Errorf(
					"get fleet before going online: %w",
					err,
				)
			}
			if fleet == nil {
				return ErrFleetInactive
			}
			if fleet.CompanyID != preAssignment.CompanyID ||
				fleet.BranchID != preAssignment.BranchID {
				return ErrAssignmentScopeInvalid
			}
			if !fleet.IsActive {
				return ErrFleetInactive
			}

			vehicle, err :=
				vehicleRepo.GetByID(
					ctx,
					preAssignment.VehicleID,
				)
			if errors.Is(err, repository.ErrNotFound) {
				return ErrVehicleInactive
			}
			if err != nil {
				return fmt.Errorf(
					"get vehicle before going online: %w",
					err,
				)
			}
			if vehicle == nil {
				return ErrVehicleInactive
			}
			if vehicle.CompanyID != preAssignment.CompanyID ||
				vehicle.BranchID != preAssignment.BranchID ||
				vehicle.FleetID != preAssignment.FleetID {
				return ErrAssignmentScopeInvalid
			}
			if !vehicle.IsActive {
				return ErrVehicleInactive
			}

			// Serialize operational state only after lifecycle eligibility has
			// been established under the parent/vehicle barriers.
			_, err =
				presenceRepo.GetByDriverIDForUpdate(
					ctx,
					req.UserID,
				)

			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return ErrDriverNotFound
			}
			if err != nil {
				return fmt.Errorf(
					"lock driver presence before going online: %w",
					err,
				)
			}

			// Unassignment can proceed without parent lifecycle locks. Re-read
			// after taking the presence lock so a stale assignment snapshot can
			// never be published into driver_presence.
			assignment, err :=
				assignmentRepo.GetActiveByDriver(
					ctx,
					req.UserID,
				)

			if errors.Is(
				err,
				repository.ErrNotFound,
			) {
				return ErrDriverAssignmentRequired
			}
			if err != nil {
				return fmt.Errorf(
					"get active driver assignment before going online: %w",
					err,
				)
			}

			if assignment == nil ||
				assignment.ID == "" ||
				assignment.ID != preAssignment.ID ||
				assignment.CompanyID != preAssignment.CompanyID ||
				assignment.BranchID != preAssignment.BranchID ||
				assignment.FleetID != preAssignment.FleetID ||
				assignment.VehicleID != preAssignment.VehicleID {

				return ErrDriverAssignmentRequired
			}

			// Reconcile the validated assignment into driver_presence. This is
			// guarded against BUSY/active-trip state.
			attached, err :=
				presenceRepo.AttachAssignmentIfIdle(
					ctx,
					req.UserID,
					assignment.CompanyID,
					assignment.BranchID,
					assignment.VehicleID,
					assignment.ID,
				)

			if err != nil {
				return fmt.Errorf(
					"attach active assignment before going online: %w",
					err,
				)
			}

			if !attached {
				return ErrDriverAvailabilityLocked
			}

			// Compliance is the final dispatchability gate after assignment
			// reconciliation. Production evaluates through transaction-bound
			// repositories while the authoritative driver row remains locked.
			compliance := s.compliance
			if s.complianceForDB != nil {
				compliance = s.complianceForDB(tx)
			}
			if compliance == nil {
				return errors.New(
					"driver compliance service is not configured",
				)
			}

			eligible, err := compliance.IsEligible(
				ctx,
				authoritativeDriver.ID,
				time.Now().UTC(),
			)
			if err != nil {
				return fmt.Errorf(
					"evaluate driver compliance before going online: %w",
					err,
				)
			}
			if !eligible {
				return ErrDriverComplianceRequired
			}

			// ---------------------------------------------------------
			// 4. Transition to ONLINE + AVAILABLE.
			//
			// Guard the final state transition as well. Both this and
			// assignment reconciliation belong to the same transaction.
			// ---------------------------------------------------------

			updated, err :=
				presenceRepo.UpdateAvailabilityIfIdle(
					ctx,
					req.UserID,
					StatusAvailable,
					true,
				)

			if err != nil {
				return fmt.Errorf(
					"mark driver available while going online: %w",
					err,
				)
			}

			if !updated {
				return ErrDriverAvailabilityLocked
			}

			return nil
		},
	)
}
