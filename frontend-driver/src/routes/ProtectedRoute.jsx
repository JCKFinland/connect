import { Navigate, Outlet, useLocation } from "react-router";

import { useAuth } from "../auth/AuthContext";

export function ProtectedRoute() {
  const { isAuthenticated, isBootstrapping } = useAuth();
  const location = useLocation();

  if (isBootstrapping) {
    return <div className="page-status">Loading CONNECT Driver...</div>;
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return <Outlet />;
}
