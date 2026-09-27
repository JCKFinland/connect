import { apiRequest } from "./client";

export function getActiveDriverTrip() {
  return apiRequest("/driver/trip", {
    authenticated: true,
  });
}

export function updateTripStatus(tripId, status) {
  return apiRequest(`/trips/${tripId}/status`, {
    method: "PATCH",
    authenticated: true,
    body: {
      status,
    },
  });
}

export function completeTrip(tripId) {
  return apiRequest(`/trips/${tripId}/complete`, {
    method: "POST",
    authenticated: true,
    body: {},
  });
}

export function recordTripLocation(tripId, location) {
  return apiRequest(`/trips/${tripId}/locations`, {
    method: "POST",
    authenticated: true,
    body: location,
  });
}

export async function getDriverTripHistory({ limit = 20, offset = 0 } = {}) {
  const statuses = ["COMPLETED", "CANCELLED"];

  const responses = await Promise.all(
    statuses.map((status) =>
      apiRequest(
        `/trips?status=${encodeURIComponent(status)}&limit=${limit}&offset=${offset}`,
        {
          authenticated: true,
        },
      ),
    ),
  );

  const trips = responses
    .flatMap((response) => response?.data ?? [])
    .sort((a, b) => {
      const aTime = new Date(
        a.completed_at || a.cancelled_at || a.created_at,
      ).getTime();

      const bTime = new Date(
        b.completed_at || b.cancelled_at || b.created_at,
      ).getTime();

      return bTime - aTime;
    });

  return trips.slice(0, limit);
}
