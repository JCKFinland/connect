package dispatch

import vehicleeligibility "github.com/JCKFinland/connect/backend/internal/services/vehicle_eligibility"

// Preserve the existing dispatch call sites while sharing the
// vehicle-matching policy with FLEX.
func isVehicleEligible(requestedType, vehicleType string) bool {
	return vehicleeligibility.IsEligible(requestedType, vehicleType)
}
