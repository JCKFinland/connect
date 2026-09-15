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

export function registerRequest({
  email,
  password,
  firstName,
  lastName,
  phone,
}) {
  return apiRequest("/auth/register", {
    method: "POST",
    body: {
      email,
      password,
      first_name: firstName,
      last_name: lastName,
      phone,
    },
  });
}
