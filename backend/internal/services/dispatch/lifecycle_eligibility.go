package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	drivercompliance "github.com/JCKFinland/connect/backend/internal/services/driver_compliance"
	"github.com/jackc/pgx/v5"
)

// dispatchLifecycleExpectation optionally pins eligibility to the hierarchy
// recorded in an offer. New offers must pin AssignmentID; callers that
// intentionally validate current eligibility without an offer may omit it.
type dispatchLifecycleExpectation struct {
	AssignmentID string
	CompanyID    string
	BranchID     string
	FleetID      string
	VehicleID    string
}

// lockAndValidateDispatchLifecycle acquires hierarchy locks BEFORE the driver
// row lock, then re-reads authoritative state in the same transaction.
//
// A false result is business ineligibility, not an infrastructure error.
// The caller must acquire the presence row lock only AFTER this function.
//
// IMPORTANT: In a multi-candidate transaction, call this inside a candidate
// savepoint. If it returns false, roll back to that savepoint before trying
// another candidate; otherwise earlier locks remain held and can invert the
// hierarchy-before-driver lock order.
//
// The caller must not already hold a driver or presence row lock when invoking
// this function. All acquired locks remain held until transaction end, or until
// rollback to the caller's savepoint.
func lockAndValidateDispatchLifecycle(
	ctx context.Context,
	tx pgx.Tx,
	driverUserID string,
	expected *dispatchLifecycleExpectation,
	at time.Time,
) (bool, error) {
	if tx == nil || driverUserID == "" {
		return false, fmt.Errorf("dispatch lifecycle requires transaction and driver user ID")
	}

	assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(tx)
	initial, err := assignments.GetActiveByDriver(ctx, driverUserID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read dispatch lifecycle assignment: %w", err)
	}
	if !validDispatchAssignment(initial, driverUserID, expected) {
		return false, nil
	}

	for _, target := range []struct{ kind, id string }{
		{"company", initial.CompanyID},
		{"branch", initial.BranchID},
		{"fleet", initial.FleetID},
		{"vehicle", initial.VehicleID},
	} {
		if err := postgresrepo.AcquireTransactionAdvisorySharedLock(
			ctx, tx, target.kind+":"+target.id,
		); err != nil {
			return false, fmt.Errorf("lock dispatch %s lifecycle: %w", target.kind, err)
		}
	}

	drivers := postgresrepo.NewDriverRepositoryWithDB(tx)
	profile, err := drivers.GetByUserID(ctx, driverUserID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve dispatch driver profile: %w", err)
	}
	if profile == nil || profile.ID == "" {
		return false, nil
	}

	lockedProfileID := profile.ID
	locked, err := drivers.TryLockByID(ctx, lockedProfileID)
	if err != nil {
		return false, fmt.Errorf("lock dispatch driver profile: %w", err)
	}
	if !locked {
		return false, nil
	}

	current, err := assignments.GetActiveByDriver(ctx, driverUserID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("recheck dispatch assignment: %w", err)
	}
	if !sameDispatchAssignment(initial, current) ||
		!validDispatchAssignment(current, driverUserID, expected) {
		return false, nil
	}

	profile, err = drivers.GetByUserID(ctx, driverUserID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("recheck dispatch driver profile: %w", err)
	}
	if profile == nil || profile.ID != lockedProfileID || !profile.IsActive || !profile.IsVerified ||
		profile.Status != "ACTIVE" ||
		profile.CompanyID != current.CompanyID ||
		profile.BranchID != current.BranchID {
		return false, nil
	}

	companies := postgresrepo.NewCompanyRepositoryWithDB(tx)
	company, err := companies.GetByID(ctx, current.CompanyID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read dispatch company: %w", err)
	}
	if company == nil || !company.IsActive {
		return false, nil
	}

	branches := postgresrepo.NewBranchRepositoryWithDB(tx)
	branch, err := branches.GetByID(ctx, current.BranchID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read dispatch branch: %w", err)
	}
	if branch == nil || !branch.IsActive || branch.CompanyID != current.CompanyID {
		return false, nil
	}

	fleets := postgresrepo.NewFleetRepositoryWithDB(tx)
	fleet, err := fleets.GetByID(ctx, current.FleetID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read dispatch fleet: %w", err)
	}
	if fleet == nil || !fleet.IsActive ||
		fleet.CompanyID != current.CompanyID || fleet.BranchID != current.BranchID {
		return false, nil
	}

	vehicles := postgresrepo.NewVehicleRepositoryWithDB(tx)
	vehicle, err := vehicles.GetByID(ctx, current.VehicleID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read dispatch vehicle: %w", err)
	}
	if vehicle == nil || !vehicle.IsActive ||
		vehicle.CompanyID != current.CompanyID ||
		vehicle.BranchID != current.BranchID ||
		vehicle.FleetID != current.FleetID {
		return false, nil
	}

	compliance := drivercompliance.NewService(drivercompliance.Dependencies{
		Drivers:   drivers,
		Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(tx),
	})
	eligible, err := compliance.IsEligible(ctx, profile.ID, at)
	if err != nil {
		return false, fmt.Errorf("evaluate dispatch regulatory compliance: %w", err)
	}
	return eligible, nil
}

func validDispatchAssignment(
	a *models.DriverAssignment,
	driverUserID string,
	expected *dispatchLifecycleExpectation,
) bool {
	if a == nil || a.ID == "" || a.DriverID != driverUserID ||
		a.CompanyID == "" || a.BranchID == "" ||
		a.FleetID == "" || a.VehicleID == "" {
		return false
	}
	if expected == nil {
		return true
	}
	return (expected.AssignmentID == "" || a.ID == expected.AssignmentID) &&
		a.CompanyID == expected.CompanyID &&
		a.BranchID == expected.BranchID &&
		a.FleetID == expected.FleetID &&
		a.VehicleID == expected.VehicleID
}

func sameDispatchAssignment(before, after *models.DriverAssignment) bool {
	return before != nil && after != nil &&
		before.ID == after.ID &&
		before.DriverID == after.DriverID &&
		before.CompanyID == after.CompanyID &&
		before.BranchID == after.BranchID &&
		before.FleetID == after.FleetID &&
		before.VehicleID == after.VehicleID
}
