import { API_BASE_URL } from "../config/api";
import { getAccessToken } from "../auth/accessTokenStore";
import { refreshAccessToken } from "../auth/session";

export class ApiError extends Error {
  constructor(message, { status = 0, data = null } = {}) {
    super(message);

    this.name = "ApiError";
    this.status = status;
    this.data = data;
  }
}

async function readResponseBody(response) {
  const contentType = response.headers.get("content-type") ?? "";

  if (contentType.includes("application/json")) {
    return response.json();
  }

  const text = await response.text();

  return text || null;
}

function buildHeaders({ headers, body, authenticated }) {
  const result = new Headers(headers);

  if (
    body !== undefined &&
    body !== null &&
    !(body instanceof FormData) &&
    !result.has("Content-Type")
  ) {
    result.set("Content-Type", "application/json");
  }

  if (authenticated) {
    const accessToken = getAccessToken();

    if (accessToken) {
      result.set("Authorization", `Bearer ${accessToken}`);
    }
  }

  return result;
}

function prepareBody(body) {
  if (
    body !== undefined &&
    body !== null &&
    !(body instanceof FormData) &&
    typeof body !== "string"
  ) {
    return JSON.stringify(body);
  }

  return body;
}

async function performRequest(
  path,
  { method, headers, body, authenticated, signal },
) {
  const requestHeaders = buildHeaders({
    headers,
    body,
    authenticated,
  });

  return fetch(`${API_BASE_URL}${path}`, {
    method,
    headers: requestHeaders,
    body: prepareBody(body),
    credentials: "include",
    signal,
  });
}

export async function refreshRequest() {
  const response = await fetch(`${API_BASE_URL}/auth/refresh`, {
    method: "POST",
    credentials: "include",
  });

  const data = await readResponseBody(response);

  if (!response.ok) {
    throw new ApiError(data?.message ?? "Unable to refresh session", {
      status: response.status,
      data,
    });
  }

  return data;
}

export async function apiRequest(
  path,
  { method = "GET", headers, body, authenticated = false, signal } = {},
) {
  const options = {
    method,
    headers,
    body,
    authenticated,
    signal,
  };

  let response = await performRequest(path, options);

  if (authenticated && response.status === 401) {
    await refreshAccessToken(refreshRequest);

    response = await performRequest(path, options);
  }

  const data = await readResponseBody(response);

  if (!response.ok) {
    throw new ApiError(data?.message ?? "Request failed", {
      status: response.status,
      data,
    });
  }

  return data;
}
