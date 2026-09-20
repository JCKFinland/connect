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
