import { Navigate, Outlet } from "react-router";

import { useAuth } from "../auth/AuthContext";

const DRIVER_ROLE = "DRIVER";

export function DriverRoute() {
  const { user } = useAuth();

  const hasDriverRole =
    Array.isArray(user?.roles) && user.roles.includes(DRIVER_ROLE);

  if (!hasDriverRole) {
    return <Navigate to="/onboarding" replace />;
  }

  return <Outlet />;
}
