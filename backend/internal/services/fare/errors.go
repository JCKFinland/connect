package fare

import "errors"

var (
	ErrInvalidTripID = errors.New(
		"trip ID is required",
	)

	ErrInvalidDistance = errors.New(
		"distance cannot be negative",
	)

	ErrInvalidDuration = errors.New(
		"duration cannot be negative",
	)

	ErrInvalidWaitingDuration = errors.New(
		"waiting duration cannot be negative",
	)

	ErrInvalidPricing = errors.New(
		"pricing values cannot be negative",
	)

	ErrInvalidSurgeMultiplier = errors.New(
		"surge multiplier must be at least 1",
	)

	ErrInvalidCurrency = errors.New(
		"currency is required",
	)

	ErrInvalidPricingVersion = errors.New(
		"pricing version is required",
	)
)
