package models

import "time"

// TripLocation represents an immutable GPS/location sample
// recorded during a trip.
//
// Trip locations are raw operational evidence. They are not themselves
// authoritative fare measurements; the trip meter service derives
// distance, duration, and waiting time from these samples.
type TripLocation struct {
	ID string `db:"id" json:"id"`

	TripID   string `db:"trip_id" json:"trip_id"`
	DriverID string `db:"driver_id" json:"driver_id"`

	Latitude  float64 `db:"latitude" json:"latitude"`
	Longitude float64 `db:"longitude" json:"longitude"`

	Altitude       *float64 `db:"altitude" json:"altitude,omitempty"`
	SpeedKMH       *float64 `db:"speed_kmh" json:"speed_kmh,omitempty"`
	Heading        *int16   `db:"heading" json:"heading,omitempty"`
	AccuracyMeters *float64 `db:"accuracy_meters" json:"accuracy_meters,omitempty"`

	RecordedAt time.Time `db:"recorded_at" json:"recorded_at"`
}
