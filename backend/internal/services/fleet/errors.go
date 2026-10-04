package fleet

import "errors"

var (
	ErrFleetNotFound     = errors.New("fleet not found")
	ErrFleetHasVehicles  = errors.New("fleet contains non-archived vehicles")
	ErrDriverNotEligible = errors.New("driver is not eligible to access fleets")
)

var ErrFleetHasActiveVehicles = errors.New(
	"fleet cannot be deactivated while it contains active vehicles",
)

var ErrFleetBranchInactive = errors.New(
	"fleet cannot be reactivated while its branch is inactive",
)
