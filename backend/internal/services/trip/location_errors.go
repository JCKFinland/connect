package trip

import "errors"

var (
	ErrTripLocationAccessDenied = errors.New(
		"trip location access denied",
	)

	ErrTripNotInProgress = errors.New(
		"trip is not in progress",
	)

	ErrInvalidTripLocation = errors.New(
		"invalid trip location",
	)

	ErrTripLocationTimestamp = errors.New(
		"invalid trip location timestamp",
	)
)
