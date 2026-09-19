import { clearAccessToken, setAccessToken } from "./accessTokenStore";

let refreshPromise = null;

export async function refreshAccessToken(refreshRequest) {
  if (!refreshPromise) {
    refreshPromise = (async () => {
      try {
        const response = await refreshRequest();

        const accessToken = response?.data?.access_token;

        if (!accessToken) {
          throw new Error("Refresh response did not contain an access token");
        }

        setAccessToken(accessToken);

        return accessToken;
      } catch (error) {
        clearAccessToken();
        throw error;
      } finally {
        refreshPromise = null;
      }
    })();
  }

  return refreshPromise;
}
