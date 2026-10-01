package dateutil

import "time"

// Before reports whether value's calendar date is before reference's
// calendar date. The time-of-day components are intentionally ignored.
//
// This is intended for values backed by PostgreSQL DATE columns, where
// year, month, and day are authoritative and must not be interpreted as
// an instant in time.
func Before(value, reference time.Time) bool {
	valueDate := time.Date(
		value.Year(),
		value.Month(),
		value.Day(),
		0, 0, 0, 0,
		time.UTC,
	)

	referenceDate := time.Date(
		reference.Year(),
		reference.Month(),
		reference.Day(),
		0, 0, 0, 0,
		time.UTC,
	)

	return valueDate.Before(referenceDate)
}
