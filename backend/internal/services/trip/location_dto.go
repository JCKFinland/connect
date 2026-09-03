package trip

import "time"

// RecordLocationRequest contains driver-device GPS evidence.
//
// DriverID is deliberately absent. The authenticated user's ID is
// authoritative and must be supplied separately by the server.
type RecordLocationRequest struct {
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
	Altitude       *float64  `json:"altitude,omitempty"`
	SpeedKMH       *float64  `json:"speed_kmh,omitempty"`
	Heading        *int      `json:"heading,omitempty"`
	AccuracyMeters *float64  `json:"accuracy_meters,omitempty"`
	RecordedAt     time.Time `json:"recorded_at"`
}
