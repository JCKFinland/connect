function formatTripDate(trip) {
    const value =
      trip.completed_at ||
      trip.cancelled_at ||
      trip.created_at;

    if (!value) {
      return "Date unavailable";
    }

    const date = new Date(value);

    if (Number.isNaN(date.getTime())) {
      return "Date unavailable";
    }

    return new Intl.DateTimeFormat("fi-FI", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(date);
  }

  function formatDistance(trip) {
    if (Number.isFinite(trip.actual_distance_km)) {
      return `${trip.actual_distance_km.toFixed(1)} km`;
    }

    if (Number.isFinite(trip.actual_distance_meters)) {
      return `${(trip.actual_distance_meters / 1000).toFixed(1)} km`;
    }

    return null;
  }

  export function TripHistoryCard({ trips, loading, error }) {
    return (
      <div className="driver-status-card">
        <h2>Trip history</h2>

        {loading ? (
          <p className="muted">Loading trip history...</p>
        ) : error ? (
          <p className="error-message">{error}</p>
        ) : trips.length === 0 ? (
          <p className="muted">No completed or cancelled trips yet.</p>
        ) : (
          <div className="trip-history-list">
            {trips.map((trip) => {
              const distance = formatDistance(trip);

              return (
                <div className="trip-history-row" key={trip.id}>
                  <div className="trip-history-main">
                    <div className="trip-history-heading">
                      <strong>{trip.status}</strong>
                      <span className="muted">{formatTripDate(trip)}</span>
                    </div>

                    <span>
                      {trip.pickup_address || "Pickup unavailable"}
                      {" → "}
                      {trip.dropoff_address || "Destination unavailable"}
                    </span>

                    {distance && (
                      <span className="muted">{distance}</span>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    );
  }