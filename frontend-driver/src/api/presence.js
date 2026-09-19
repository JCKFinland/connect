import { apiRequest } from "./client";

export function getCurrentPresence() {
  return apiRequest("/driver/presence", {
    authenticated: true,
  });
}

export function goOnline() {
  return apiRequest("/driver/online", {
    method: "POST",
    authenticated: true,
    body: {},
  });
}

export function goOffline() {
  return apiRequest("/driver/offline", {
    method: "POST",
    authenticated: true,
    body: {},
  });
}

export function sendHeartbeat({
  latitude,
  longitude,
  heading,
  speed,
  accuracy,
}) {
  return apiRequest("/driver/heartbeat", {
    method: "POST",
    authenticated: true,
    body: {
      latitude,
      longitude,
      heading,
      speed,
      accuracy,
    },
  });
}
