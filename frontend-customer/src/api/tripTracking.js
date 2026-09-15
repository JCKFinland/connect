import { API_BASE_URL } from "../config/api";
import { getAccessToken } from "../auth/accessTokenStore";
import { refreshAccessToken } from "../auth/session";
import { ApiError, apiRequest, refreshRequest } from "./client";

export function getTripLocationsRequest(tripId, { signal } = {}) {
  return apiRequest(`/trips/${encodeURIComponent(tripId)}/locations`, {
    authenticated: true,
    signal,
  });
}

async function openStreamRequest(tripId, signal) {
  const accessToken = getAccessToken();

  const headers = new Headers({
    Accept: "text/event-stream",
  });

  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }

  return fetch(`${API_BASE_URL}/trips/${encodeURIComponent(tripId)}/stream`, {
    method: "GET",
    headers,
    credentials: "include",
    signal,
  });
}

export async function openTripStream(tripId, { signal } = {}) {
  let response = await openStreamRequest(tripId, signal);

  if (response.status === 401) {
    await refreshAccessToken(refreshRequest);

    response = await openStreamRequest(tripId, signal);
  }

  if (!response.ok) {
    let data = null;

    try {
      data = await response.json();
    } catch {
      // A failed stream response is not
      // guaranteed to contain JSON.
    }

    throw new ApiError(data?.message ?? "Unable to open trip stream", {
      status: response.status,
      data,
    });
  }

  if (!response.body) {
    throw new ApiError("Trip stream response did not contain a body", {
      status: response.status,
    });
  }

  return response.body;
}
