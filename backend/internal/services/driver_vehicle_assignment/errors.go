package driver_vehicle_assignment

import "errors"

var (

	// ErrAssignmentNotFound indicates that the requested assignment does not exist.
	ErrAssignmentNotFound = errors.New(
		"driver-vehicle assignment not found",
	)

	// ErrDriverAlreadyAssigned indicates that the driver already has an active assignment.
	ErrDriverAlreadyAssigned = errors.New(
		"driver already has an active vehicle assignment",
	)

	// ErrVehicleAlreadyAssigned indicates that the vehicle already has an active driver.
	ErrVehicleAlreadyAssigned = errors.New(
		"vehicle already has an active driver assignment",
	)

	// ErrAssignmentAlreadyReleased indicates that the assignment has already ended.
	ErrAssignmentAlreadyReleased = errors.New(
		"assignment has already been released",
	)

	// ErrInvalidAssignment indicates invalid assignment data.
	ErrInvalidAssignment = errors.New(
		"invalid driver-vehicle assignment",
	)
)