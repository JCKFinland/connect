import { useEffect, useRef, useState } from "react";
import { recordTripLocation } from "../api/trips";

const MINIMUM_SAMPLE_INTERVAL_MS = 5000;

export function useTripLocationTracking(trip) {
  const geolocationSupported = "geolocation" in navigator;

  const [recordingError, setRecordingError] = useState("");
  const watchIdRef = useRef(null);
  const recordingRef = useRef(false);
  const lastRecordedAtRef = useRef(0);

  useEffect(() => {
    if (trip?.status !== "IN_PROGRESS" || !geolocationSupported) {
      return;
    }

    let cancelled = false;

    watchIdRef.current = navigator.geolocation.watchPosition(
      async (position) => {
        if (cancelled || recordingRef.current) {
          return;
        }

        const { coords, timestamp } = position;

        if (
          lastRecordedAtRef.current > 0 &&
          timestamp - lastRecordedAtRef.current < MINIMUM_SAMPLE_INTERVAL_MS
        ) {
          return;
        }

        // Ignore GPS samples that the backend would reject.
        if (typeof coords.accuracy !== "number" || coords.accuracy > 50) {
          return;
        }

        const location = {
          latitude: coords.latitude,
          longitude: coords.longitude,
          accuracy_meters: coords.accuracy,
          recorded_at: new Date(timestamp).toISOString(),
        };

        if (typeof coords.altitude === "number") {
          location.altitude = coords.altitude;
        }

        // Browser speed is metres/second.
        // CONNECT expects kilometres/hour.
        if (typeof coords.speed === "number" && coords.speed >= 0) {
          location.speed_kmh = coords.speed * 3.6;
        }

        if (
          typeof coords.heading === "number" &&
          coords.heading >= 0 &&
          coords.heading < 360
        ) {
          location.heading = Math.round(coords.heading);
        }

        recordingRef.current = true;

        try {
          await recordTripLocation(trip.id, location);

          lastRecordedAtRef.current = timestamp;

          if (!cancelled) {
            setRecordingError("");
          }
        } catch (error) {
          if (!cancelled) {
            setRecordingError(
              error.message || "Unable to record trip location.",
            );
          }
        } finally {
          recordingRef.current = false;
        }
      },
      (error) => {
        if (!cancelled) {
          setRecordingError(
            error.message || "Unable to access driver location.",
          );
        }
      },
      {
        enableHighAccuracy: true,
        maximumAge: 5000,
        timeout: 10000,
      },
    );

    return () => {
      cancelled = true;

      if (watchIdRef.current !== null) {
        navigator.geolocation.clearWatch(watchIdRef.current);
        watchIdRef.current = null;
      }

      recordingRef.current = false;
      lastRecordedAtRef.current = 0;
    };
  }, [trip?.id, trip?.status, geolocationSupported]);

  const locationError =
    trip?.status === "IN_PROGRESS" && !geolocationSupported
      ? "Location services are not supported by this browser."
      : recordingError;

  return {
    locationError,
  };
}
