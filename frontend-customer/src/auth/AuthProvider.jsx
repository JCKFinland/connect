import { useCallback, useEffect, useMemo, useState } from "react";

import {
  getCurrentUserRequest,
  loginRequest,
  logoutRequest,
  registerRequest,
} from "../api/auth";
import { clearAccessToken, setAccessToken } from "./accessTokenStore";
import { AuthContext } from "./AuthContext";
import { bootstrapSession } from "./bootstrapSession";

export default function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [isBootstrapping, setIsBootstrapping] = useState(true);

  const loadCurrentUser = useCallback(async () => {
    const response = await getCurrentUserRequest();

    const currentUser = response?.data;

    if (!currentUser) {
      throw new Error("Current user response did not contain user data");
    }

    setUser(currentUser);

    return currentUser;
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function restoreSession() {
      try {
        const currentUser = await bootstrapSession();

        if (!cancelled) {
          setUser(currentUser);
        }
      } catch {
        if (!cancelled) {
          setUser(null);
        }
      } finally {
        if (!cancelled) {
          setIsBootstrapping(false);
        }
      }
    }

    void restoreSession();

    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(
    async ({ email, password }) => {
      const response = await loginRequest({
        email,
        password,
      });

      const accessToken = response?.data?.access_token;

      if (!accessToken) {
        clearAccessToken();

        throw new Error("Login response did not contain an access token");
      }

      setAccessToken(accessToken);

      try {
        return await loadCurrentUser();
      } catch (error) {
        clearAccessToken();
        setUser(null);

        throw error;
      }
    },
    [loadCurrentUser],
  );

  const register = useCallback(
    async ({ email, password, firstName, lastName, phone }) =>
      registerRequest({
        email,
        password,
        firstName,
        lastName,
        phone,
      }),
    [],
  );

  const logout = useCallback(async () => {
    try {
      await logoutRequest();
    } finally {
      clearAccessToken();
      setUser(null);
    }
  }, []);

  const value = useMemo(
    () => ({
      user,
      isAuthenticated: user !== null,
      isBootstrapping,
      login,
      logout,
      register,
    }),
    [user, isBootstrapping, login, logout, register],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
