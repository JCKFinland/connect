package dispatch

import "math"

const earthRadiusKM = 6371.0

func distanceKM(
	lat1 float64,
	lon1 float64,
	lat2 float64,
	lon2 float64,
) float64 {

	lat1Rad := degreesToRadians(lat1)
	lon1Rad := degreesToRadians(lon1)
	lat2Rad := degreesToRadians(lat2)
	lon2Rad := degreesToRadians(lon2)

	dLat := lat2Rad - lat1Rad
	dLon := lon2Rad - lon1Rad

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*
			math.Cos(lat2Rad)*
			math.Sin(dLon/2)*
			math.Sin(dLon/2)

	c := 2 * math.Atan2(
		math.Sqrt(a),
		math.Sqrt(1-a),
	)

	return earthRadiusKM * c
}

func degreesToRadians(
	degrees float64,
) float64 {
	return degrees * math.Pi / 180
}
