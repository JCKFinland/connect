package assignment

import "errors"

var (

	// Authenticated user has no driver registration.
	ErrDriverNotFound = errors.New("driver not found")

	// Requested vehicle does not exist.
	ErrVehicleNotFound = errors.New("vehicle not found")

	// Vehicle must belong to the authenticated driver's company and branch.
	ErrVehicleOutsideDriverScope = errors.New("vehicle is outside driver organizational scope")

	// Vehicle must be operationally active before it can be assigned.
	ErrVehicleInactive = errors.New("vehicle is inactive")

	// Driver already has an active assignment.
	ErrDriverAlreadyAssigned = errors.New("driver already assigned")

	// Vehicle already has an active assignment.
	ErrVehicleAlreadyAssigned = errors.New("vehicle already assigned")

	// Assignment not found.
	ErrAssignmentNotFound = errors.New("assignment not found")

	// Driver must be online before assignment.
	ErrDriverOffline = errors.New("driver is offline")

	// Driver must be AVAILABLE before assignment.
	ErrDriverUnavailable = errors.New("driver is not available")
)
