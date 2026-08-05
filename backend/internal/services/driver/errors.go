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
)