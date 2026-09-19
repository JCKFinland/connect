import { useEffect } from "react";
import { sendHeartbeat } from "../api/presence";

const HEARTBEAT_INTERVAL_MS = 30_000;

export function useDriverHeartbeat(isOnline) {
  useEffect(() => {
    if (!isOnline) {
      return undefined;
    }

    let cancelled = false;
    let intervalId = null;

    function sendCurrentLocation() {
      if (!navigator.geolocation) {
        return;
      }

      navigator.geolocation.getCurrentPosition(
        async (position) => {
          if (cancelled) {
            return;
          }

          const { latitude, longitude, heading, speed, accuracy } =
            position.coords;

          try {
            await sendHeartbeat({
              latitude,
              longitude,
              heading: heading ?? 0,
              speed: speed ?? 0,
              accuracy: accuracy ?? 0,
            });
          } catch {
            // Presence state and API errors remain authoritative.
            // A failed heartbeat will be retried on the next interval.
          }
        },
        () => {
          // Location permission/errors are handled by the dashboard later.
        },
        {
          enableHighAccuracy: true,
          timeout: 10_000,
          maximumAge: 15_000,
        },
      );
    }

    sendCurrentLocation();

    intervalId = window.setInterval(
      sendCurrentLocation,
      HEARTBEAT_INTERVAL_MS,
    );

    return () => {
      cancelled = true;

      if (intervalId !== null) {
        window.clearInterval(intervalId);
      }
    };
  }, [isOnline]);
}
