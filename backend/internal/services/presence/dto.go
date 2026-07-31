package presence

import "time"

type GoOnlineRequest struct {
	DriverID string `json:"driver_id"`
}

type HeartbeatRequest struct {
	DriverID string `json:"driver_id"`

	Latitude float64 `json:"latitude"`

	Longitude float64 `json:"longitude"`

	Heading float64 `json:"heading"`

	Speed float64 `json:"speed"`

	Accuracy float64 `json:"accuracy"`
}

type UpdateAvailabilityRequest struct {
	DriverID string `json:"driver_id"`

	Status string `json:"status"`
}

type GoOfflineRequest struct {
	DriverID string `json:"driver_id"`
}

type PresenceResponse struct {
	DriverID string `json:"driver_id"`

	IsOnline bool `json:"is_online"`

	AvailabilityStatus string `json:"availability_status"`

	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
}
