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