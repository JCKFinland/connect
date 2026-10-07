package presence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

var ErrDriverAvailabilityLocked = errors.New(
	"driver availability cannot be changed while committed to an active trip",
)

var ErrDriverOffline = errors.New(
	"driver must be online before changing availability",
)

var ErrInvalidAvailabilityStatus = errors.New(
	"invalid driver availability status",
)

func (s *Service) UpdateAvailability(
	ctx context.Context,
	req UpdateAvailabilityRequest,
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

	// ---------------------------------------------------------
	// 1. Validate driver-controlled availability state before
	//    starting any persistence work.
	// ---------------------------------------------------------

	switch req.Status {

	case StatusAvailable,
		StatusBreak,
		StatusOffDuty:

		// Driver-controlled availability states.

	default:
		return ErrInvalidAvailabilityStatus
	}

	// ---------------------------------------------------------
	// 2. Lock presence and mutate availability atomically.
	// ---------------------------------------------------------

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			presenceRepo :=
				postgresrepo.NewDriverPresenceRepositoryWithDB(
					tx,
				)

			var preAssignment *models.DriverAssignment
			var authoritativeDriverID string

			// AVAILABLE is an operational re-enable transition. Apply the
			// same authoritative lifecycle eligibility barrier as GoOnline
			// before allowing the driver to accept dispatch work.
			if req.Status == StatusAvailable {
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

				var err error
				preAssignment, err =
					assignmentRepo.GetActiveByDriver(
						ctx,
						req.UserID,
					)

				if errors.Is(err, repository.ErrNotFound) {
					return ErrDriverAssignmentRequired
				}
				if err != nil {
					return fmt.Errorf(
						"get active driver assignment before becoming available: %w",
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
							"acquire %s lifecycle lock before becoming available: %w",
							target.name,
							err,
						)
					}
				}

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
						"get authoritative driver before becoming available: %w",
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

				authoritativeDriverID = authoritativeDriver.ID

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
						"get company before becoming available: %w",
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
						"get branch before becoming available: %w",
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
						"get fleet before becoming available: %w",
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
						"get vehicle before becoming available: %w",
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
			}

			// ---------------------------------------------------------
			// Lock the driver's lifecycle row first.
			//
			// This lets us distinguish:
			//
			//   missing presence
			//       -> ErrDriverNotFound
			//
			//   BUSY / active trip
			//       -> ErrDriverAvailabilityLocked
			//
			// The same presence row is also used as the serialization
			// point by dispatch and assignment lifecycle operations.
			// ---------------------------------------------------------

			presence, err :=
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
					"lock driver presence before availability update: %w",
					err,
				)
			}

			if presence == nil || !presence.IsOnline {
				return ErrDriverOffline
			}

			// AVAILABLE may have waited behind a lifecycle transition after
			// pre-reading its assignment. Re-read after presence serialization
			// so a closed or replaced assignment cannot enable dispatchability.
			if req.Status == StatusAvailable {
				assignmentRepo :=
					postgresrepo.NewDriverAssignmentRepositoryWithDB(
						tx,
					)

				assignment, err :=
					assignmentRepo.GetActiveByDriver(
						ctx,
						req.UserID,
					)

				if errors.Is(err, repository.ErrNotFound) {
					return ErrDriverAssignmentRequired
				}
				if err != nil {
					return fmt.Errorf(
						"recheck active driver assignment before becoming available: %w",
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

				// Compliance is the final dispatchability gate after lifecycle,
				// presence, and assignment state have been authoritatively validated.
				if s.compliance == nil {
					return errors.New(
						"driver compliance service is not configured",
					)
				}

				eligible, err := s.compliance.IsEligible(
					ctx,
					authoritativeDriverID,
					time.Now().UTC(),
				)
				if err != nil {
					return fmt.Errorf(
						"evaluate driver compliance before becoming available: %w",
						err,
					)
				}
				if !eligible {
					return ErrDriverComplianceRequired
				}
			}

			// ---------------------------------------------------------
			// Apply the requested manual state only if the driver
			// remains operationally idle.
			// ---------------------------------------------------------

			updated, err :=
				presenceRepo.UpdateAvailabilityIfIdle(
					ctx,
					req.UserID,
					req.Status,
					true,
				)

			if err != nil {
				return fmt.Errorf(
					"update driver availability: %w",
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
