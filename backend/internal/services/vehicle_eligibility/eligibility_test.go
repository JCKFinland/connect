package vehicle_eligibility

import "testing"

func TestIsEligible(t *testing.T) {
	tests := []struct {
		name          string
		requestedType string
		vehicleType   string
		want          bool
	}{
		{
			name:          "standard accepts sedan",
			requestedType: "STANDARD",
			vehicleType:   "SEDAN",
			want:          true,
		},
		{
			name:          "van accepts van",
			requestedType: "VAN",
			vehicleType:   "VAN",
			want:          true,
		},
		{
			name:          "standard rejects van",
			requestedType: "STANDARD",
			vehicleType:   "VAN",
			want:          false,
		},
		{
			name:          "van rejects sedan",
			requestedType: "VAN",
			vehicleType:   "SEDAN",
			want:          false,
		},
		{
			name:          "normalizes whitespace and case",
			requestedType: " standard ",
			vehicleType:   " sedan ",
			want:          true,
		},
		{
			name:          "rejects unknown requested type",
			requestedType: "PREMIUM",
			vehicleType:   "SEDAN",
			want:          false,
		},
		{
			name:          "rejects unknown vehicle type",
			requestedType: "STANDARD",
			vehicleType:   "SUV",
			want:          false,
		},
		{
			name:          "rejects empty requested type",
			requestedType: "",
			vehicleType:   "SEDAN",
			want:          false,
		},
		{
			name:          "rejects empty vehicle type",
			requestedType: "STANDARD",
			vehicleType:   "",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEligible(tt.requestedType, tt.vehicleType)
			if got != tt.want {
				t.Errorf(
					"IsEligible(%q, %q) = %t, want %t",
					tt.requestedType,
					tt.vehicleType,
					got,
					tt.want,
				)
			}
		})
	}
}
