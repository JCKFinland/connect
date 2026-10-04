package fleet

import "errors"

var (
	ErrFleetNotFound     = errors.New("fleet not found")
	ErrFleetHasVehicles  = errors.New("fleet contains non-archived vehicles")
	ErrDriverNotEligible = errors.New("driver is not eligible to access fleets")
)
