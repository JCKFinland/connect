import { useState } from "react";
import { Link, Navigate, useNavigate } from "react-router";

import { useAuth } from "../auth/AuthContext";

export default function RegisterPage() {
  const { isAuthenticated, isBootstrapping, register } = useAuth();

  const navigate = useNavigate();

  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
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
    return <Navigate to="/" replace />;
  }

  async function handleSubmit(event) {
    event.preventDefault();

    setError("");
    setIsSubmitting(true);

    try {
      await register({
        email: email.trim(),
        password,
        firstName: firstName.trim(),
        lastName: lastName.trim(),
        phone: phone.trim(),
      });

      navigate("/login", {
        replace: true,
        state: {
          registrationSuccess: true,
        },
      });
    } catch (requestError) {
      setError(requestError?.message ?? "Unable to create account");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <section>
      <h2>Create account</h2>

      <form onSubmit={handleSubmit}>
        <div>
          <label htmlFor="firstName">First name</label>

          <input
            id="firstName"
            name="firstName"
            type="text"
            autoComplete="given-name"
            required
            maxLength={100}
            value={firstName}
            onChange={(event) => setFirstName(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="lastName">Last name</label>

          <input
            id="lastName"
            name="lastName"
            type="text"
            autoComplete="family-name"
            required
            maxLength={100}
            value={lastName}
            onChange={(event) => setLastName(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="email">Email</label>

          <input
            id="email"
            name="email"
            type="email"
            autoComplete="email"
            required
            maxLength={255}
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="phone">Phone</label>

          <input
            id="phone"
            name="phone"
            type="tel"
            autoComplete="tel"
            required
            maxLength={30}
            value={phone}
            onChange={(event) => setPhone(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="password">Password</label>

          <input
            id="password"
            name="password"
            type="password"
            autoComplete="new-password"
            required
            minLength={8}
            maxLength={72}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>

        {error ? <p role="alert">{error}</p> : null}

        <button type="submit" disabled={isSubmitting}>
          {isSubmitting ? "Creating account..." : "Create account"}
        </button>
      </form>

      <p>
        Already have an account? <Link to="/login">Login</Link>
      </p>
    </section>
  );
}
