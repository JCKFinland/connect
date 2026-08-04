package vehicle

import "errors"

var (
	// ErrVehicleNotFound indicates that the requested vehicle does not exist.
	ErrVehicleNotFound = errors.New("vehicle not found")

	// ErrDuplicateRegistrationNumber indicates that the registration number already exists.
	ErrDuplicateRegistrationNumber = errors.New("vehicle registration number already exists")

	// ErrDuplicateVIN indicates that the VIN already exists.
	ErrDuplicateVIN = errors.New("vehicle VIN already exists")

	// ErrInvalidVehicle indicates that the supplied vehicle data is invalid.
	ErrInvalidVehicle = errors.New("invalid vehicle")
)