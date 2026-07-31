package assignment

import "errors"

var (

	// Driver already has an active assignment.
	ErrDriverAlreadyAssigned =
		errors.New("driver already assigned")

	// Vehicle already has an active assignment.
	ErrVehicleAlreadyAssigned =
		errors.New("vehicle already assigned")

	// Assignment not found.
	ErrAssignmentNotFound =
		errors.New("assignment not found")

	// Driver must be online before assignment.
	ErrDriverOffline =
		errors.New("driver is offline")

	// Driver must be AVAILABLE before assignment.
	ErrDriverUnavailable =
		errors.New("driver is not available")
)