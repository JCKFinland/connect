import { Link, useLocation } from "react-router";

import { useAuth } from "../auth/AuthContext";

function HomePage() {
  const { isAuthenticated } = useAuth();
  const location = useLocation();

  const rideRequestCreated = location.state?.rideRequestCreated === true;

  const rideRequestId = location.state?.rideRequestId;

  return (
    <section>
      <h2>CONNECT Customer</h2>

      <p>Book and track your taxi journey.</p>

      {rideRequestCreated ? (
        <div role="status">
          <p>Your ride request has been created successfully.</p>

          {rideRequestId ? <p>Ride request: {rideRequestId}</p> : null}
        </div>
      ) : null}

      {isAuthenticated ? (
        <p>
          <Link to="/book">Book a ride</Link>
        </p>
      ) : null}
    </section>
  );
}

export default HomePage;
