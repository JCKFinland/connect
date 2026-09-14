import { useParams } from "react-router";

function TripPage() {
  const { tripId } = useParams();

  return (
    <section>
      <h2>Trip</h2>

      <p>Trip ID: {tripId}</p>
    </section>
  );
}

export default TripPage;
