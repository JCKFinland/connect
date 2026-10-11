import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { getRideRequest, getTripByRideRequest } from "../api/rideRequests";

const POLL_INTERVAL_MS = 5000;
const TERMINAL_STATUSES = new Set([
  "CANCELLED",
  "EXPIRED",
]);

const ACTIVE_STATUSES = new Set([
  "PENDING",
  "DISPATCHING",
  "ACCEPTED",
]);

export default function RideRequestPage() {
  const { rideRequestId } = useParams();
  const navigate = useNavigate();

  const [rideRequest, setRideRequest] = useState(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState("");
  const [isTerminal, setIsTerminal] = useState(false);
  const [isStopped, setIsStopped] = useState(false);

  useEffect(() => {
    if (!rideRequestId) {
      return undefined;
    }

    const controller = new AbortController();
    let cancelled = false;
    let timerId;

    async function poll() {
      if (cancelled) return;

      try {
        const request = await getRideRequest(rideRequestId, {
          signal: controller.signal,
        });

        if (cancelled) return;

        if (!request) {
          throw new Error("Ride request response did not contain data");
        }

        setRideRequest(request);
        setError("");
        setIsLoading(false);

        const status = String(request.status ?? "").toUpperCase();

        try {
          const trip = await getTripByRideRequest(rideRequestId, {
            signal: controller.signal,
          });

          if (cancelled) return;

          if (!trip?.id) {
            throw new Error("Trip discovery response did not contain a trip ID");
          }

          navigate(`/trips/${encodeURIComponent(trip.id)}`, {
            replace: true,
          });
          return;
        } catch (tripError) {
          if (cancelled || controller.signal.aborted) return;

          // HTTP 404 means no trip exists for this ride request yet.
          if (tripError?.status !== 404) {
            throw tripError;
          }
        }

        if (TERMINAL_STATUSES.has(status)) {
          setIsTerminal(true);
          setIsStopped(true);
          return;
        }

        if (!ACTIVE_STATUSES.has(status)) {
          setError(`Unexpected ride request status: ${status || "UNKNOWN"}`);
          setIsStopped(true);
          return;
        }
      } catch (requestError) {
        if (cancelled || controller.signal.aborted) return;

        setError(requestError?.message ?? "Unable to check ride request");
        setIsLoading(false);

        if ([401, 403, 404].includes(requestError?.status)) {
          setIsStopped(true);
          return;
        }
      }

      if (!cancelled && !controller.signal.aborted) {
        timerId = window.setTimeout(poll, POLL_INTERVAL_MS);
      }
    }

    void poll();

    return () => {
      cancelled = true;
      controller.abort();
      window.clearTimeout(timerId);
    };
  }, [rideRequestId, navigate]);

  const status = String(rideRequest?.status ?? "").toUpperCase();

  return (
    <section>
      <h2>Your ride request</h2>

      <p>Ride request ID: {rideRequestId}</p>

      {!rideRequestId ? (
        <p role="alert">Ride request ID is missing</p>
      ) : null}

      {rideRequestId && isLoading ? (
        <p role="status">Loading your ride request...</p>
      ) : null}

      {error ? <p role="alert">{error}</p> : null}

      {rideRequest ? (
        <>
          <p>Status: {status || "PENDING"}</p>

          <p>Pickup: {rideRequest.pickup_address}</p>

          <p>Destination: {rideRequest.destination_address}</p>

          {isTerminal ? (
            <p role="status">
              This ride request has ended with status {status}.
            </p>
          ) : !isStopped ? (
            <p role="status">
              Waiting for a driver. This page checks automatically for an
              accepted trip.
            </p>
          ) : null}
        </>
      ) : null}

      <p>
        <Link to="/">Return to home</Link>
      </p>
    </section>
  );
}
