export function ActiveTripCard({ trip }) {
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
    </div>
  );
}
