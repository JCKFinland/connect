export function DispatchOfferCard({ offer, responding, onAccept, onReject }) {
  if (!offer) {
    return (
      <div className="dispatch-offer-card">
        <h2>Ride offers</h2>
        <p className="muted">No pending ride offer.</p>
      </div>
    );
  }

  const ride = offer.ride ?? {};

  return (
    <div className="dispatch-offer-card">
      <h2>New ride offer</h2>

      <p>
        <strong>Pickup:</strong> {ride.pickup_address || "Not available"}
      </p>

      <p>
        <strong>Destination:</strong>{" "}
        {ride.destination_address || "Not available"}
      </p>

      <p>
        <strong>Passengers:</strong> {ride.passenger_count ?? 0}
      </p>

      {ride.requested_vehicle_type && (
        <p>
          <strong>Vehicle:</strong> {ride.requested_vehicle_type}
        </p>
      )}

      {ride.notes && (
        <p>
          <strong>Notes:</strong> {ride.notes}
        </p>
      )}

      <div className="dispatch-offer-actions">
        <button
          type="button"
          disabled={responding}
          onClick={() => onAccept(offer.id)}
        >
          {responding ? "Responding..." : "Accept ride"}
        </button>

        <button
          type="button"
          disabled={responding}
          onClick={() => onReject(offer.id)}
        >
          Reject ride
        </button>
      </div>
    </div>
  );
}
