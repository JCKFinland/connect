import { getCurrentUserRequest } from "../api/auth";
import { refreshRequest } from "../api/client";
import { clearAccessToken } from "./accessTokenStore";
import { refreshAccessToken } from "./session";

let bootstrapPromise = null;

export function bootstrapSession() {
  if (!bootstrapPromise) {
    bootstrapPromise = (async () => {
      try {
        await refreshAccessToken(refreshRequest);

        const response = await getCurrentUserRequest();

        const currentUser = response?.data;

        if (!currentUser) {
          throw new Error("Current user response did not contain user data");
        }

        return currentUser;
      } catch (error) {
        clearAccessToken();
        throw error;
      } finally {
        bootstrapPromise = null;
      }
    })();
  }

  return bootstrapPromise;
}
