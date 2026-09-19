package routing

import "errors"

var (
	ErrInvalidOrigin = errors.New(
		"invalid route origin",
	)

	ErrInvalidDestination = errors.New(
		"invalid route destination",
	)

	ErrRouteUnavailable = errors.New(
		"route unavailable",
	)
)
