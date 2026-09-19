package fare_estimate

import "errors"

var (
	ErrInvalidServiceCategoryID = errors.New(
		"service category ID is required",
	)

	ErrInvalidBookingCompanyID = errors.New(
		"booking company ID is required",
	)
)
