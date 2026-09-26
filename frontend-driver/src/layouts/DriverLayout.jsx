import { NavLink, Outlet, useNavigate } from "react-router";

import { useAuth } from "../auth/AuthContext";

export function DriverLayout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  async function handleLogout() {
    await logout();
    navigate("/login", { replace: true });
  }

  return (
    <div className="app-shell">
      <header className="app-header">
        <div>
          <strong>CONNECT Driver</strong>
          <div className="muted">
            {user?.first_name} {user?.last_name}
          </div>
        </div>

        <nav className="app-navigation" aria-label="Driver navigation">
          <NavLink to="/" end>
            Dashboard
          </NavLink>

          <NavLink to="/vehicles/register">Register vehicle</NavLink>
        </nav>

        <button type="button" onClick={handleLogout}>
          Log out
        </button>
      </header>

      <main className="app-main">
        <Outlet />
      </main>
    </div>
  );
}