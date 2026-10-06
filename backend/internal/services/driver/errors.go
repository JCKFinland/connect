package driver

import "errors"

var (

	// ErrDriverNotFound indicates that the requested driver does not exist.
	ErrDriverNotFound = errors.New(
		"driver not found",
	)

	// ErrDriverAlreadyExists indicates that a duplicate driver was detected.
	ErrDriverAlreadyExists = errors.New(
		"driver already exists",
	)

	// ErrInvalidDriver indicates that the supplied driver information is invalid.
	ErrInvalidDriver = errors.New(
		"invalid driver",
	)

	// ErrDriverNotPendingVerification indicates that the driver application
	// is not currently awaiting verification.
	ErrDriverNotPendingVerification = errors.New(
		"driver is not pending verification",
	)

	// ErrDriverOperationallyActive indicates that the driver still has an
	// online operational presence and cannot be archived.
	ErrDriverOperationallyActive = errors.New(
		"driver is operationally active",
	)

	// ErrDriverHasActiveAssignment indicates that the driver still has an
	// active vehicle assignment and cannot be archived.
	ErrDriverHasActiveAssignment = errors.New(
		"driver has active assignment",
	)

	// ErrDriverHasActiveTrip indicates that the driver still has a
	// non-terminal trip and cannot be archived.
	ErrDriverHasActiveTrip = errors.New(
		"driver has active trip",
	)
)
