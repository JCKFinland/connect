package dispatch

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	drivereligibility "github.com/JCKFinland/connect/backend/internal/services/driver_eligibility"
	"github.com/jackc/pgx/v5"
)

// Preserve the existing dispatch expectation contract.
type dispatchLifecycleExpectation = drivereligibility.LifecycleExpectation

// lockAndValidateDispatchLifecycle retains the existing dispatch entry point.
//
// The shared implementation preserves hierarchy-before-driver locking,
// authoritative assignment revalidation, and regulatory compliance checks.
//
// Callers must acquire presence locks only after this function.
func lockAndValidateDispatchLifecycle(
	ctx context.Context,
	tx pgx.Tx,
	driverUserID string,
	expected *dispatchLifecycleExpectation,
	at time.Time,
) (bool, error) {
	return drivereligibility.LockAndValidateLifecycle(
		ctx,
		tx,
		driverUserID,
		expected,
		at,
	)
}

func validDispatchAssignment(
	assignment *models.DriverAssignment,
	driverUserID string,
	expected *dispatchLifecycleExpectation,
) bool {
	return drivereligibility.ValidAssignment(
		assignment,
		driverUserID,
		expected,
	)
}

func sameDispatchAssignment(
	before, after *models.DriverAssignment,
) bool {
	return drivereligibility.SameAssignment(before, after)
}
