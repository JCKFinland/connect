package models

import (
	"encoding/json"
	"time"
)

// TripEvent represents an immutable operational/audit event
// associated with a trip.
type TripEvent struct {
	ID string `db:"id" json:"id"`

	TripID string `db:"trip_id" json:"trip_id"`

	EventType string `db:"event_type" json:"event_type"`

	PerformedByUserID *string `db:"performed_by_user_id" json:"performed_by_user_id,omitempty"`

	Latitude  *float64 `db:"latitude" json:"latitude,omitempty"`
	Longitude *float64 `db:"longitude" json:"longitude,omitempty"`

	Metadata json.RawMessage `db:"metadata" json:"metadata,omitempty"`

	OccurredAt time.Time `db:"occurred_at" json:"occurred_at"`
}
