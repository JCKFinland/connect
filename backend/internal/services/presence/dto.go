package presence

import "time"

type GoOnlineRequest struct {
    UserID string `json:"-"`
}

type HeartbeatRequest struct {
    UserID string `json:"-"`

    Latitude  float64 `json:"latitude"`
    Longitude float64 `json:"longitude"`
    Heading   float64 `json:"heading"`
    Speed     float64 `json:"speed"`
    Accuracy  float64 `json:"accuracy"`
}

type UpdateAvailabilityRequest struct {
    UserID string `json:"-"`

    Status string `json:"status"`
}

type GoOfflineRequest struct {
    UserID string `json:"-"`
}

type PresenceResponse struct {
    DriverID           string     `json:"driver_id"`
    IsOnline           bool       `json:"is_online"`
    AvailabilityStatus string     `json:"availability_status"`
    LastHeartbeatAt    *time.Time `json:"last_heartbeat_at,omitempty"`
}