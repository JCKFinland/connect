package dispatch

import "strings"

func isVehicleEligible(
	requestedType string,
	vehicleType string,
) bool {

	requestedType = strings.ToUpper(
		strings.TrimSpace(requestedType),
	)

	vehicleType = strings.ToUpper(
		strings.TrimSpace(vehicleType),
	)

	switch requestedType {
	case "STANDARD":
		return vehicleType == "SEDAN"

	case "VAN":
		return vehicleType == "VAN"

	default:
		return false
	}
}
