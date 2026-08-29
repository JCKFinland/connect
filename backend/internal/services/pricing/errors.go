package pricing

import "errors"

var (
	ErrInvalidCompanyID = errors.New("company ID is required")

	ErrInvalidServiceCategoryID = errors.New("service category ID is required")

	ErrInvalidEffectiveTime = errors.New("effective time is required")

	ErrPricingProfileNotFound = errors.New("effective pricing profile not found")
)
