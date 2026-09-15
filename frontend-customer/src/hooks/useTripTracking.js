import { useEffect, useState } from "react";

import { getTripLocationsRequest, openTripStream } from "../api/tripTracking";
import { consumeSSE } from "../services/sse";

function mergeLocations(existingLocations, incomingLocations) {
  const byID = new Map();

  for (const location of existingLocations) {
    if (location?.id) {
      byID.set(location.id, location);
    }
  }

  for (const location of incomingLocations) {
    if (location?.id) {
      byID.set(location.id, location);
    }
  }

  return Array.from(byID.values()).sort(
    (left, right) =>
      new Date(left.recorded_at).getTime() -
      new Date(right.recorded_at).getTime(),
  );
}

export default function useTripTracking(tripId) {
  const [locations, setLocations] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!tripId) {
      return undefined;
    }

    const controller = new AbortController();

    let cancelled = false;

    async function startTracking() {
      try {
        setError("");

        const stream = await openTripStream(tripId, {
          signal: controller.signal,
        });

        let historyRequested = false;

        await consumeSSE(stream, {
          signal: controller.signal,

          onEvent(event) {
            if (cancelled) {
              return;
            }

            if (event.type === "connected") {
              setIsConnected(true);

              if (!historyRequested) {
                historyRequested = true;

                void loadHistory();
              }

              return;
            }

            if (event.type === "trip.location" && event.data?.id) {
              setLocations((current) => mergeLocations(current, [event.data]));
            }
          },
        });

        if (!cancelled && !controller.signal.aborted) {
          setIsConnected(false);
          setError("Live trip connection ended");
        }
      } catch (requestError) {
        if (cancelled || controller.signal.aborted) {
          return;
        }

        setIsConnected(false);
        setError(requestError?.message ?? "Unable to track trip");
        setIsLoading(false);
      }
    }

    async function loadHistory() {
      try {
        const response = await getTripLocationsRequest(tripId, {
          signal: controller.signal,
        });

        const history = Array.isArray(response?.data) ? response.data : [];

        if (!cancelled) {
          setLocations((current) => mergeLocations(current, history));
        }
      } catch (requestError) {
        if (cancelled || controller.signal.aborted) {
          return;
        }

        setError(requestError?.message ?? "Unable to load trip locations");
      } finally {
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    }

    void startTracking();

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [tripId]);

  const latestLocation =
    locations.length > 0 ? locations[locations.length - 1] : null;

  return {
    locations,
    latestLocation,
    isLoading,
    isConnected,
    error,
  };
}
