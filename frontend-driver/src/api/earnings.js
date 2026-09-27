import { apiRequest } from "./client";

export function getDriverEarnings() {
  return apiRequest("/driver/earnings", {
    authenticated: true,
  });
}