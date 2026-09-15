import { Link, Outlet, useNavigate } from "react-router";

import { useAuth } from "../auth/AuthContext";

export default function CustomerLayout() {
  const { user, isAuthenticated, isBootstrapping, logout } = useAuth();

  const navigate = useNavigate();

  async function handleLogout() {
    try {
      await logout();
    } finally {
      navigate("/login", {
        replace: true,
      });
    }
  }

  return (
    <div>
      <header>
        <h1>
          <Link to="/">CONNECT</Link>
        </h1>

        {!isBootstrapping ? (
          <nav>
            {isAuthenticated ? (
              <>
                <span>{user?.first_name ?? user?.email}</span>

                <button type="button" onClick={handleLogout}>
                  Logout
                </button>
              </>
            ) : (
              <>
                <Link to="/login">Login</Link>{" "}
                <Link to="/register">Create account</Link>
              </>
            )}
          </nav>
        ) : null}
      </header>

      <main>
        <Outlet />
      </main>
    </div>
  );
}
