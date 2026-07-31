package presence

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

const (
	StatusOffline   = "OFFLINE"
	StatusAvailable = "AVAILABLE"
	StatusBusy      = "BUSY"
	StatusBreak     = "BREAK"
)

// AttachAssignment creates or updates the driver's
// presence after a successful assignment.
func (s *Service) AttachAssignment(
	ctx context.Context,
	assignment *models.DriverAssignment,
) error {

	p, err := s.presence.GetByDriverID(
		ctx,
		assignment.DriverID,
	)

	// No presence record yet → create one.
	if err == repository.ErrNotFound {

		now := time.Now().UTC()

		branchID := assignment.BranchID
		vehicleID := assignment.VehicleID
		assignmentID := assignment.ID

		return s.presence.Create(
			ctx,
			&models.DriverPresence{
				DriverID: assignment.DriverID,

				CompanyID: assignment.CompanyID,

				BranchID: &branchID,

				VehicleID: &vehicleID,

				AssignmentID: &assignmentID,

				IsOnline: false,

				AvailabilityStatus: StatusOffline,

				LastHeartbeatAt: &now,
			},
		)
	}

	if err != nil {
		return err
	}

	// Presence exists → update assignment information.
	branchID := assignment.BranchID
	vehicleID := assignment.VehicleID
	assignmentID := assignment.ID

	p.CompanyID = assignment.CompanyID
	p.BranchID = &branchID
	p.VehicleID = &vehicleID
	p.AssignmentID = &assignmentID
	return s.presence.Update(
		ctx,
		p,
	)
}

// DetachAssignment removes the assignment from
// the driver's presence.
func (s *Service) DetachAssignment(
	ctx context.Context,
	driverID string,
) error {

	p, err := s.presence.GetByDriverID(
		ctx,
		driverID,
	)

	if err != nil {
		return err
	}

	p.AssignmentID = nil

	p.VehicleID = nil

	p.IsOnline = false

	p.AvailabilityStatus = StatusOffline

	return s.presence.Update(
		ctx,
		p,
	)
}
