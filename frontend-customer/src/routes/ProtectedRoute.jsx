import { Navigate, Outlet, useLocation } from "react-router";

import { useAuth } from "../auth/AuthContext";

export default function ProtectedRoute() {
  const { isAuthenticated, isBootstrapping } = useAuth();

  const location = useLocation();

  if (isBootstrapping) {
    return (
      <main>
        <p>Restoring your CONNECT session...</p>
      </main>
    );
  }

  if (!isAuthenticated) {
    return (
      <Navigate
        to="/login"
        replace
        state={{
          from: location,
        }}
      />
    );
  }

  return <Outlet />;
}
