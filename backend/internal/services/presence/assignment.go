package presence

import "context"

const (
	StatusOffline   = "OFFLINE"
	StatusAvailable = "AVAILABLE"
	StatusBusy      = "BUSY"
	StatusBreak     = "BREAK"
	StatusOffDuty   = "OFF_DUTY"
	StatusSuspended = "SUSPENDED"
)

// DetachAssignment removes assignment-related presence state only when the
// driver is not committed to an active trip.
//
// This method is suitable for transaction-backed presence repositories.
// Assignment lifecycle callers must treat ErrDriverAvailabilityLocked as
// a failed unassignment and roll back their surrounding transaction.
func (s *Service) DetachAssignment(
	ctx context.Context,
	driverID string,
) error {

	updated, err :=
		s.presence.DetachAssignmentIfIdle(
			ctx,
			driverID,
		)

	if err != nil {
		return err
	}

	if !updated {
		return ErrDriverAvailabilityLocked
	}

	return nil
}
