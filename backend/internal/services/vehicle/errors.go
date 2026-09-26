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

	// ErrDriverNotEligible indicates that the authenticated user does not
	// have an active, verified driver profile.
	ErrDriverNotEligible = errors.New("driver is not eligible to register vehicles")

	// ErrInvalidFleet indicates that the selected fleet is inactive or does
	// not belong to the authenticated driver's company and branch.
	ErrInvalidFleet = errors.New("invalid fleet for driver")
)
