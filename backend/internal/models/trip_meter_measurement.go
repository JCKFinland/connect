package models

import "time"

// TripMeterMeasurement is the immutable authoritative measurement
// snapshot produced from trip evidence and used during fare completion.
type TripMeterMeasurement struct {
	ID                     string    `db:"id" json:"id"`
	TripID                 string    `db:"trip_id" json:"trip_id"`
	MeasurementSource      string    `db:"measurement_source" json:"measurement_source"`
	AlgorithmVersion       string    `db:"algorithm_version" json:"algorithm_version"`
	DistanceMeters         int64     `db:"distance_meters" json:"distance_meters"`
	DurationSeconds        int64     `db:"duration_seconds" json:"duration_seconds"`
	WaitingDurationSeconds int64     `db:"waiting_duration_seconds" json:"waiting_duration_seconds"`
	AcceptedSamples        int       `db:"accepted_samples" json:"accepted_samples"`
	RejectedSamples        int       `db:"rejected_samples" json:"rejected_samples"`
	RejectedSegments       int       `db:"rejected_segments" json:"rejected_segments"`
	MeasuredAt             time.Time `db:"measured_at" json:"measured_at"`
	CreatedAt              time.Time `db:"created_at" json:"created_at"`
}
