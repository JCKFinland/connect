package dispatch

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

// claimDispatchCandidate isolates candidate locks in a PostgreSQL savepoint.
// Rejected candidates release every lock before the next candidate is tried.
// A selected candidate retains its locks in the parent transaction.
func claimDispatchCandidate(ctx context.Context, tx pgx.Tx, driverUserID string, vehicleAllowed func(*models.Vehicle) bool, at time.Time, heartbeatTimeout time.Duration, previouslyOffered map[string]struct{}) (*models.DriverPresence, *models.DriverAssignment, error) {
	candidateTx, err := tx.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin dispatch candidate savepoint: %w", err)
	}
	defer func() { _ = candidateTx.Rollback(ctx) }()

	eligible, err := lockAndValidateDispatchLifecycle(ctx, candidateTx, driverUserID, nil, at)
	if err != nil {
		return nil, nil, err
	}
	if !eligible {
		return nil, nil, nil
	}

	drivers := postgresrepo.NewDriverRepositoryWithDB(candidateTx)
	profile, err := drivers.GetByUserID(ctx, driverUserID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read locked dispatch profile: %w", err)
	}
	if profile == nil || profile.ID == "" {
		return nil, nil, nil
	}
	if previouslyOffered != nil {
		if _, exists := previouslyOffered[profile.ID]; exists {
			return nil, nil, nil
		}
	}

	presenceRepo := postgresrepo.NewDriverPresenceRepositoryWithDB(candidateTx)
	lockedPresence, err := presenceRepo.GetAvailableByDriverIDForUpdate(ctx, driverUserID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lock dispatch candidate presence: %w", err)
	}
	if lockedPresence == nil || lockedPresence.LastHeartbeatAt == nil {
		return nil, nil, nil
	}
	age := at.Sub(lockedPresence.LastHeartbeatAt.UTC())
	if age < 0 || age > heartbeatTimeout {
		return nil, nil, nil
	}

	assignments := postgresrepo.NewDriverAssignmentRepositoryWithDB(candidateTx)
	assignment, err := assignments.GetActiveByDriver(ctx, driverUserID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("recheck locked dispatch assignment: %w", err)
	}
	if !validDispatchAssignment(assignment, driverUserID, nil) {
		return nil, nil, nil
	}
	// The live presence must refer to this exact active assignment and scope.
	// A valid assignment alone cannot authorize a stale or detached presence row.
	if lockedPresence.AssignmentID == nil || *lockedPresence.AssignmentID != assignment.ID ||
		lockedPresence.VehicleID == nil || *lockedPresence.VehicleID != assignment.VehicleID ||
		lockedPresence.CompanyID != assignment.CompanyID ||
		lockedPresence.BranchID == nil || *lockedPresence.BranchID != assignment.BranchID {
		return nil, nil, nil
	}
	vehicles := postgresrepo.NewVehicleRepositoryWithDB(candidateTx)
	vehicle, err := vehicles.GetByID(ctx, assignment.VehicleID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read locked dispatch vehicle: %w", err)
	}
	if vehicle == nil || !vehicle.IsActive || !vehicleAllowed(vehicle) {
		return nil, nil, nil
	}

	if err := candidateTx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("release selected dispatch candidate savepoint: %w", err)
	}
	return lockedPresence, assignment, nil
}
