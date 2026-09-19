import { useCallback, useEffect, useMemo, useState } from "react";

import {
  getCurrentUserRequest,
  loginRequest,
  logoutRequest,
} from "../api/auth";
import { clearAccessToken, setAccessToken } from "./accessTokenStore";
import { AuthContext } from "./AuthContext";
import { bootstrapSession } from "./bootstrapSession";

const DRIVER_ROLE = "DRIVER";

function hasDriverRole(user) {
  return Array.isArray(user?.roles) && user.roles.includes(DRIVER_ROLE);
}

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [isBootstrapping, setIsBootstrapping] = useState(true);

  const clearSession = useCallback(() => {
    clearAccessToken();
    setUser(null);
  }, []);

  const loadCurrentDriver = useCallback(async () => {
    const response = await getCurrentUserRequest();
    const currentUser = response?.data;

    if (!currentUser) {
      throw new Error("Current user response did not contain user data");
    }

    if (!hasDriverRole(currentUser)) {
      throw new Error("This account does not have driver access.");
    }

    setUser(currentUser);

    return currentUser;
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function restoreSession() {
      try {
        const currentUser = await bootstrapSession();

        if (!hasDriverRole(currentUser)) {
          throw new Error("This account does not have driver access.");
        }

        if (!cancelled) {
          setUser(currentUser);
        }
      } catch {
        if (!cancelled) {
          clearSession();
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
  }, [clearSession]);

  const login = useCallback(
    async ({ email, password }) => {
      const response = await loginRequest({
        email,
        password,
      });

      const accessToken = response?.data?.access_token;

      if (!accessToken) {
        clearSession();
        throw new Error("Login response did not contain an access token");
      }

      setAccessToken(accessToken);

      try {
        return await loadCurrentDriver();
      } catch (error) {
        try {
          await logoutRequest();
        } finally {
          clearSession();
        }

        throw error;
      }
    },
    [clearSession, loadCurrentDriver],
  );

  const logout = useCallback(async () => {
    try {
      await logoutRequest();
    } finally {
      clearSession();
    }
  }, [clearSession]);

  const value = useMemo(
    () => ({
      user,
      isAuthenticated: user !== null,
      isBootstrapping,
      login,
      logout,
    }),
    [user, isBootstrapping, login, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
