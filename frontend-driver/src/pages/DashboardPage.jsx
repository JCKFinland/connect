import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext";
import { getCurrentPresence, goOffline, goOnline } from "../api/presence";

export function DashboardPage() {
  const { user } = useAuth();

  const [presence, setPresence] = useState(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [error, setError] = useState("");

  async function loadPresence() {
    try {
      setError("");

      const response = await getCurrentPresence();
      setPresence(response?.data ?? null);
    } catch (err) {
      setError(err.message || "Unable to load driver status.");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    let cancelled = false;

    async function initializePresence() {
      try {
        const response = await getCurrentPresence();

        if (!cancelled) {
          setPresence(response?.data ?? null);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err.message || "Unable to load driver status.");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    initializePresence();

    return () => {
      cancelled = true;
    };
  }, []);

  async function handleOnline() {
    setUpdating(true);
    setError("");

    try {
      await goOnline();
      await loadPresence();
    } catch (err) {
      setError(err.message || "Unable to go online.");
    } finally {
      setUpdating(false);
    }
  }

  async function handleOffline() {
    setUpdating(true);
    setError("");

    try {
      await goOffline();
      await loadPresence();
    } catch (err) {
      setError(err.message || "Unable to go offline.");
    } finally {
      setUpdating(false);
    }
  }

  return (
    <section className="page">
      <h1>Driver dashboard</h1>
      <p>Welcome, {user?.first_name || user?.email}.</p>

      <div className="driver-status-card">
        <h2>Driver status</h2>

        {loading ? (
          <p className="muted">Loading status...</p>
        ) : (
          <>
            <p>
              Status:{" "}
              <strong>{presence?.is_online ? "Online" : "Offline"}</strong>
            </p>

            <p>
              Availability:{" "}
              <strong>{presence?.availability_status || "OFFLINE"}</strong>
            </p>

            {error && <p className="error-message">{error}</p>}

            <div className="driver-status-actions">
              {presence?.is_online ? (
                <button
                  type="button"
                  disabled={updating}
                  onClick={handleOffline}
                >
                  {updating ? "Updating..." : "Go offline"}
                </button>
              ) : (
                <button
                  type="button"
                  disabled={updating}
                  onClick={handleOnline}
                >
                  {updating ? "Updating..." : "Go online"}
                </button>
              )}
            </div>
          </>
        )}
      </div>
    </section>
  );
}
