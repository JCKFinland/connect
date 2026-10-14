package vehicle_eligibility

import "strings"

// IsEligible applies CONNECT's current requested-vehicle-type
// matching policy. Both standard dispatch and FLEX must use
// the same authoritative rule.
func IsEligible(requestedType, vehicleType string) bool {
	requestedType = strings.ToUpper(strings.TrimSpace(requestedType))
	vehicleType = strings.ToUpper(strings.TrimSpace(vehicleType))

	switch requestedType {
	case "STANDARD":
		return vehicleType == "SEDAN"
	case "VAN":
		return vehicleType == "VAN"
	default:
		return false
	}
}
