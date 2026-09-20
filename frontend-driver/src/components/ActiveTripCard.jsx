export function ActiveTripCard({
  trip,
  updating = false,
  onStatusUpdate,
  onComplete,
}) {
  if (!trip) {
    return (
      <div className="driver-status-card">
        <h2>Active trip</h2>
        <p className="muted">No active trip.</p>
      </div>
    );
  }

  return (
    <div className="driver-status-card">
      <h2>Active trip</h2>

      <p>
        Status: <strong>{trip.status}</strong>
      </p>

      <p>
        <strong>Pickup</strong>
        <br />
        {trip.pickup_address || "Pickup address unavailable"}
      </p>

      <p>
        <strong>Destination</strong>
        <br />
        {trip.dropoff_address || "Destination address unavailable"}
      </p>

      {trip.passenger_note && (
        <p>
          <strong>Passenger note</strong>
          <br />
          {trip.passenger_note}
        </p>
      )}

      {trip.status === "ASSIGNED" && (
        <div className="driver-status-actions">
          <button
            type="button"
            disabled={updating}
            onClick={() => onStatusUpdate?.("DRIVER_EN_ROUTE")}
          >
            {updating ? "Updating..." : "Start driving to pickup"}
          </button>
        </div>
      )}

      {trip.status === "DRIVER_EN_ROUTE" && (
        <div className="driver-status-actions">
          <button
            type="button"
            disabled={updating}
            onClick={() => onStatusUpdate?.("DRIVER_ARRIVED")}
          >
            {updating ? "Updating..." : "Arrived at pickup"}
          </button>
        </div>
      )}

      {trip.status === "DRIVER_ARRIVED" && (
        <div className="driver-status-actions">
          <button
            type="button"
            disabled={updating}
            onClick={() => onStatusUpdate?.("IN_PROGRESS")}
          >
            {updating ? "Updating..." : "Start trip"}
          </button>
        </div>
      )}

      {trip.status === "IN_PROGRESS" && (
        <div className="driver-status-actions">
          <button
            type="button"
            disabled={updating}
            onClick={() => onComplete?.()}
          >
            {updating ? "Completing..." : "Complete trip"}
          </button>
        </div>
      )}
    </div>
  );
}
