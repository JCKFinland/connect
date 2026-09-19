import { apiRequest } from "./client";

export function loginRequest({ email, password }) {
  return apiRequest("/auth/login", {
    method: "POST",
    body: {
      email,
      password,
    },
  });
}

export function logoutRequest() {
  return apiRequest("/auth/logout", {
    method: "POST",
  });
}

export function getCurrentUserRequest() {
  return apiRequest("/users/me", {
    authenticated: true,
  });
}
