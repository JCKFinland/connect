package routing

import "context"

// Coordinate identifies a geographic point.
type Coordinate struct {
	Latitude  float64
	Longitude float64
}

// RouteRequest contains the endpoints required for road-route calculation.
type RouteRequest struct {
	Origin      Coordinate
	Destination Coordinate
}

// RouteMetrics contains provider-derived road-route measurements.
type RouteMetrics struct {
	DistanceMeters  int64
	DurationSeconds int64
}

// Service resolves road-route metrics between two coordinates.
type Service interface {
	Route(
		ctx context.Context,
		request RouteRequest,
	) (*RouteMetrics, error)
}
