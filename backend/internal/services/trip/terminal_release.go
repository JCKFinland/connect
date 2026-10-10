package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/services/driver_compliance"
	"github.com/jackc/pgx/v5"
)

const driverAvailabilityOffline = "OFFLINE"

type terminalDriverLocks struct {
	driverUserID string
	assignment   *models.DriverAssignment
}

// lockTerminalTripDriver establishes lifecycle barriers before locking the
// permanent driver aggregate. Its assignment snapshot is revalidated before
// an AVAILABLE release can be authorized.
func lockTerminalTripDriver(
	ctx context.Context,
	tx pgx.Tx,
	driverUserID string,
) (*terminalDriverLocks, error) {
	locks := &terminalDriverLocks{driverUserID: driverUserID}
	if driverUserID == "" {
		return locks, nil
	}

	assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)

	assignment, err := assignments.GetActiveByDriver(ctx, driverUserID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("read terminal driver assignment: %w", err)
	}

	if err == nil && assignment != nil {
		locks.assignment = assignment

		targets := []struct {
			name string
			id   string
		}{
			{"company", assignment.CompanyID},
			{"branch", assignment.BranchID},
			{"fleet", assignment.FleetID},
			{"vehicle", assignment.VehicleID},
		}

		for _, target := range targets {
			if target.id == "" {
				continue
			}

			if err := postgresrepo.AcquireTransactionAdvisorySharedLock(
				ctx, tx, target.name+":"+target.id,
			); err != nil {
				return nil, fmt.Errorf(
					"acquire %s lifecycle lock before terminal release: %w",
					target.name, err,
				)
			}
		}
	}

	drivers := postgresrepo.NewDriverRepositoryWithDB(tx)
	if err := drivers.LockAllByUserIDForTerminalRelease(
		ctx, driverUserID,
	); err != nil {
		return nil, fmt.Errorf("lock terminal driver aggregate: %w", err)
	}

	return locks, nil
}

func sameTerminalAssignment(
	before, after *models.DriverAssignment,
) bool {
	return before != nil &&
		after != nil &&
		before.ID != "" &&
		before.ID == after.ID &&
		before.CompanyID != "" &&
		before.BranchID != "" &&
		before.FleetID != "" &&
		before.VehicleID != "" &&
		before.CompanyID == after.CompanyID &&
		before.BranchID == after.BranchID &&
		before.FleetID == after.FleetID &&
		before.VehicleID == after.VehicleID
}

// terminalReleaseTarget makes an authoritative decision inside the terminal
// transaction. Business ineligibility means OFFLINE; infrastructure errors
// roll back the terminal transition.
func terminalReleaseTarget(
	ctx context.Context,
	tx pgx.Tx,
	locks *terminalDriverLocks,
) (string, error) {
	if locks == nil || locks.driverUserID == "" {
		return driverAvailabilityOffline, nil
	}

	if locks.assignment == nil {
		return driverAvailabilityOffline, nil
	}

	assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)

	assignment, err := assignments.GetActiveByDriver(
		ctx, locks.driverUserID,
	)
	if errors.Is(err, repository.ErrNotFound) {
		return driverAvailabilityOffline, nil
	}
	if err != nil {
		return "", fmt.Errorf("revalidate terminal assignment: %w", err)
	}
	if !sameTerminalAssignment(locks.assignment, assignment) {
		return driverAvailabilityOffline, nil
	}

	drivers := postgresrepo.NewDriverRepositoryWithDB(tx)
	driver, err := drivers.GetByUserID(ctx, locks.driverUserID)
	if errors.Is(err, repository.ErrNotFound) {
		return driverAvailabilityOffline, nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve terminal driver: %w", err)
	}
	if driver == nil ||
		!driver.IsActive ||
		!driver.IsVerified ||
		driver.Status != "ACTIVE" ||
		driver.CompanyID != assignment.CompanyID ||
		driver.BranchID != assignment.BranchID {
		return driverAvailabilityOffline, nil
	}

	companies := postgresrepo.NewCompanyRepositoryWithDB(tx)
	company, err := companies.GetByID(ctx, assignment.CompanyID)
	if errors.Is(err, repository.ErrNotFound) {
		return driverAvailabilityOffline, nil
	}
	if err != nil {
		return "", fmt.Errorf("read terminal company: %w", err)
	}
	if company == nil || !company.IsActive {
		return driverAvailabilityOffline, nil
	}

	branches := postgresrepo.NewBranchRepositoryWithDB(tx)
	branch, err := branches.GetByID(ctx, assignment.BranchID)
	if errors.Is(err, repository.ErrNotFound) {
		return driverAvailabilityOffline, nil
	}
	if err != nil {
		return "", fmt.Errorf("read terminal branch: %w", err)
	}
	if branch == nil ||
		!branch.IsActive ||
		branch.CompanyID != assignment.CompanyID {
		return driverAvailabilityOffline, nil
	}

	fleets := postgresrepo.NewFleetRepositoryWithDB(tx)
	fleet, err := fleets.GetByID(ctx, assignment.FleetID)
	if errors.Is(err, repository.ErrNotFound) {
		return driverAvailabilityOffline, nil
	}
	if err != nil {
		return "", fmt.Errorf("read terminal fleet: %w", err)
	}
	if fleet == nil ||
		!fleet.IsActive ||
		fleet.CompanyID != assignment.CompanyID ||
		fleet.BranchID != assignment.BranchID {
		return driverAvailabilityOffline, nil
	}

	vehicles := postgresrepo.NewVehicleRepositoryWithDB(tx)
	vehicle, err := vehicles.GetByID(ctx, assignment.VehicleID)
	if errors.Is(err, repository.ErrNotFound) {
		return driverAvailabilityOffline, nil
	}
	if err != nil {
		return "", fmt.Errorf("read terminal vehicle: %w", err)
	}
	if vehicle == nil ||
		!vehicle.IsActive ||
		vehicle.CompanyID != assignment.CompanyID ||
		vehicle.BranchID != assignment.BranchID ||
		vehicle.FleetID != assignment.FleetID {
		return driverAvailabilityOffline, nil
	}

	compliance := driver_compliance.NewService(
		driver_compliance.Dependencies{
			Drivers:   drivers,
			Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(tx),
		},
	)

	eligibility, err := compliance.Evaluate(
		ctx, driver.ID, time.Now().UTC(),
	)
	if err != nil {
		return "", fmt.Errorf(
			"evaluate terminal driver compliance: %w", err,
		)
	}
	if eligibility == nil {
		return "", fmt.Errorf(
			"terminal driver compliance returned no eligibility decision",
		)
	}
	if !eligibility.Eligible {
		return driverAvailabilityOffline, nil
	}

	return driverAvailabilityAvailable, nil
}

// releaseTerminalTripDriver performs the release atomically with the
// terminal trip transition and its audit event.
func releaseTerminalTripDriver(
	ctx context.Context,
	tx pgx.Tx,
	locks *terminalDriverLocks,
	tripID string,
) error {
	if locks == nil || locks.driverUserID == "" {
		return nil
	}

	target, err := terminalReleaseTarget(ctx, tx, locks)
	if err != nil {
		return err
	}

	presence := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)
	released, err := presence.ReleaseFromTripWithTarget(
		ctx, locks.driverUserID, tripID, target,
	)
	if err != nil {
		return fmt.Errorf("release terminal trip driver: %w", err)
	}
	if !released {
		return fmt.Errorf(
			"release terminal trip driver: lifecycle authorization failed",
		)
	}

	return nil
}
