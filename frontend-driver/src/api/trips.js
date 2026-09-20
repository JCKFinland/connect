import { apiRequest } from "./client";

export function getActiveDriverTrip() {
  return apiRequest("/driver/trip", {
    authenticated: true,
  });
}
