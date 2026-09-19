package routing

func validateRequest(request RouteRequest) error {
	if !validCoordinate(request.Origin) {
		return ErrInvalidOrigin
	}

	if !validCoordinate(request.Destination) {
		return ErrInvalidDestination
	}

	return nil
}

func validCoordinate(coordinate Coordinate) bool {
	return coordinate.Latitude >= -90 &&
		coordinate.Latitude <= 90 &&
		coordinate.Longitude >= -180 &&
		coordinate.Longitude <= 180
}
