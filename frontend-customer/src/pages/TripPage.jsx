import { useParams } from "react-router";

import TripPayment from "../components/TripPayment";
import useTripTracking from "../hooks/useTripTracking";

export default function TripPage() {
  const { tripId } = useParams();

  const { locations, latestLocation, isLoading, isConnected, error } =
    useTripTracking(tripId);

  return (
    <section>
      <h2>Trip</h2>

      <p>Trip ID: {tripId}</p>

      <p>Live connection: {isConnected ? "Connected" : "Disconnected"}</p>

      {isLoading ? <p>Loading trip locations...</p> : null}

      {error ? <p role="alert">{error}</p> : null}

      <p>Location samples: {locations.length}</p>

      {latestLocation ? (
        <section>
          <h3>Latest driver location</h3>

          <dl>
            <dt>Latitude</dt>
            <dd>{latestLocation.latitude}</dd>

            <dt>Longitude</dt>
            <dd>{latestLocation.longitude}</dd>

            <dt>Recorded at</dt>
            <dd>{latestLocation.recorded_at}</dd>
          </dl>
        </section>
      ) : !isLoading ? (
        <p>No driver location has been recorded yet.</p>
      ) : null}
      <TripPayment tripId={tripId} />
    </section>
  );
}
