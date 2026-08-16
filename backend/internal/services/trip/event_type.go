package trip

const (
	EventDriverEnRoute = "DRIVER_EN_ROUTE"
	EventDriverArrived = "DRIVER_ARRIVED"
	EventTripStarted   = "TRIP_STARTED"
	EventTripCompleted = "TRIP_COMPLETED"
	EventTripCancelled = "TRIP_CANCELLED"
)

// eventTypeForStatus converts a trip lifecycle status into the
// corresponding immutable trip_events.event_type value.
func eventTypeForStatus(
	status string,
) (string, bool) {

	switch status {

	case StatusDriverEnRoute:
		return EventDriverEnRoute, true

	case StatusDriverArrived:
		return EventDriverArrived, true

	case StatusInProgress:
		return EventTripStarted, true

	case StatusCompleted:
		return EventTripCompleted, true

	case StatusCancelled:
		return EventTripCancelled, true

	default:
		return "", false
	}
}
