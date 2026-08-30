package tripmeter

import "math"

const earthRadiusMeters = 6371000.0

// haversineDistanceMeters calculates the great-circle distance
// between two geographic coordinates.
func haversineDistanceMeters(
	lat1 float64,
	lon1 float64,
	lat2 float64,
	lon2 float64,
) float64 {
	lat1Rad := degreesToRadians(lat1)
	lat2Rad := degreesToRadians(lat2)

	deltaLat := degreesToRadians(lat2 - lat1)
	deltaLon := degreesToRadians(lon2 - lon1)

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*
			math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*
			math.Sin(deltaLon/2)

	c := 2 * math.Atan2(
		math.Sqrt(a),
		math.Sqrt(1-a),
	)

	return earthRadiusMeters * c
}

func degreesToRadians(degrees float64) float64 {
	return degrees * math.Pi / 180
}
