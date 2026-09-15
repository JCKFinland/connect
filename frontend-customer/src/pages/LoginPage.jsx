import { useState } from "react";
import { Link, Navigate, useLocation } from "react-router";

import { useAuth } from "../auth/AuthContext";

export default function LoginPage() {
  const { isAuthenticated, isBootstrapping, login } = useAuth();

  const location = useLocation();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (isBootstrapping) {
    return (
      <section>
        <p>Restoring your CONNECT session...</p>
      </section>
    );
  }

  if (isAuthenticated) {
    const destination = location.state?.from?.pathname ?? "/";

    return <Navigate to={destination} replace />;
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
    } catch (requestError) {
      setError(requestError?.message ?? "Unable to log in");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <section>
      <h2>Login</h2>

      {location.state?.registrationSuccess ? (
        <p role="status">Account created successfully. Please log in.</p>
      ) : null}

      <form onSubmit={handleSubmit}>
        <div>
          <label htmlFor="email">Email</label>

          <input
            id="email"
            name="email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="password">Password</label>

          <input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>

        {error ? <p role="alert">{error}</p> : null}

        <button type="submit" disabled={isSubmitting}>
          {isSubmitting ? "Logging in..." : "Login"}
        </button>
      </form>

      <p>
        New to CONNECT? <Link to="/register">Create an account</Link>
      </p>
    </section>
  );
}
