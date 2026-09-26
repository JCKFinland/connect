package fleet

import "errors"

var (
	ErrFleetNotFound     = errors.New("fleet not found")
	ErrDriverNotEligible = errors.New("driver is not eligible to access fleets")
)
