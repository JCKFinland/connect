import { useAuth } from "../auth/AuthContext";

export function DashboardPage() {
  const { user } = useAuth();

  return (
    <section className="page">
      <h1>Driver dashboard</h1>
      <p>Welcome, {user?.first_name || user?.email}.</p>

      <p className="muted">Your CONNECT driver workspace is ready.</p>
    </section>
  );
}
