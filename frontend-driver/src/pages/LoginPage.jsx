import { useState } from "react";
import { Navigate, useLocation, useNavigate } from "react-router";

import { useAuth } from "../auth/AuthContext";

export function LoginPage() {
  const { isAuthenticated, isBootstrapping, login } = useAuth();

  const location = useLocation();
  const navigate = useNavigate();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (isBootstrapping) {
    return <div className="page-status">Loading CONNECT Driver...</div>;
  }

  if (isAuthenticated) {
    return <Navigate to="/" replace />;
  }

  async function handleSubmit(event) {
    event.preventDefault();

    setError("");
    setIsSubmitting(true);

    try {
      await login({
        email: email.trim(),
        password,
      });

      const destination = location.state?.from?.pathname || "/";

      navigate(destination, { replace: true });
    } catch (loginError) {
      setError(loginError?.message || "Unable to sign in.");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-card">
        <h1>CONNECT Driver</h1>
        <p>Sign in to your driver account.</p>

        <form onSubmit={handleSubmit}>
          <label>
            Email
            <input
              type="email"
              autoComplete="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              required
            />
          </label>

          <label>
            Password
            <input
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
            />
          </label>

          {error ? (
            <p className="error-message" role="alert">
              {error}
            </p>
          ) : null}

          <button type="submit" disabled={isSubmitting}>
            {isSubmitting ? "Signing in..." : "Sign in"}
          </button>
        </form>
      </section>
    </main>
  );
}
