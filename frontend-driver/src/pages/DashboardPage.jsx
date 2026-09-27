import { useEffect, useState } from "react";
import { ApiError } from "../api/client";
import {
  acceptDispatchOffer,
  getPendingDispatchOffer,
  rejectDispatchOffer,
} from "../api/dispatchOffers";
import { getCurrentPresence, goOffline, goOnline } from "../api/presence";
import {
  completeTrip,
  getActiveDriverTrip,
  getDriverTripHistory,
  updateTripStatus,
} from "../api/trips";
import { useAuth } from "../auth/AuthContext";
import { ActiveTripCard } from "../components/ActiveTripCard";
import { TripHistoryCard } from "../components/TripHistoryCard";
import { DispatchOfferCard } from "../components/DispatchOfferCard";
import { getDriverEarnings } from "../api/earnings";
import { EarningsCard } from "../components/EarningsCard";
import { useDriverHeartbeat } from "../hooks/useDriverHeartbeat";
import { useTripLocationTracking } from "../hooks/useTripLocationTracking";

const OFFER_POLL_INTERVAL_MS = 5000;

export function DashboardPage() {
  const { user } = useAuth();

  const [presence, setPresence] = useState(null);
  const [loading, setLoading] = useState(true);
  const [updating, setUpdating] = useState(false);
  const [error, setError] = useState("");

  const [pendingOffer, setPendingOffer] = useState(null);
  const [offerResponding, setOfferResponding] = useState(false);
  const [offerError, setOfferError] = useState("");

  const [activeTrip, setActiveTrip] = useState(null);
  const [tripUpdating, setTripUpdating] = useState(false);
  const [tripError, setTripError] = useState("");

  const [earnings, setEarnings] = useState(null);
  const [earningsLoading, setEarningsLoading] = useState(true);
  const [earningsError, setEarningsError] = useState("");

  const [tripHistory, setTripHistory] = useState([]);
  const [tripHistoryLoading, setTripHistoryLoading] = useState(true);
  const [tripHistoryError, setTripHistoryError] = useState("");

  useDriverHeartbeat(presence?.is_online === true);

  const { locationError } = useTripLocationTracking(activeTrip);

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

  async function loadPendingOffer() {
    try {
      const response = await getPendingDispatchOffer();

      setPendingOffer(response?.data ?? null);
      setOfferError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setPendingOffer(null);
        setOfferError("");
        return;
      }

      setOfferError(err.message || "Unable to load ride offers.");
    }
  }

  async function loadActiveTrip() {
    try {
      const response = await getActiveDriverTrip();

      setActiveTrip(response?.data ?? null);
      setTripError("");
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setActiveTrip(null);
        setTripError("");
        return;
      }

      setTripError(err.message || "Unable to load active trip.");
    }
  }

  async function loadEarnings() {
    try {
      setEarningsError("");

      const response = await getDriverEarnings();
      setEarnings(response?.data ?? null);
    } catch (err) {
      setEarningsError(err.message || "Unable to load earnings.");
    } finally {
      setEarningsLoading(false);
    }
  }

  async function loadTripHistory() {
    try {
      setTripHistoryError("");

      const trips = await getDriverTripHistory();
      setTripHistory(trips);
    } catch (err) {
      setTripHistoryError(err.message || "Unable to load trip history.");
    } finally {
      setTripHistoryLoading(false);
    }
  }

  useEffect(() => {
    let cancelled = false;

    async function initializePresence() {
      try {
        const response = await getCurrentPresence();

        if (!cancelled) {
          const currentPresence = response?.data ?? null;

          setPresence(currentPresence);

          if (currentPresence?.is_online) {
            await loadActiveTrip();
          }
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

    void (async () => {
      await loadEarnings();
    })();

    void (async () => {
      await loadTripHistory();
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!presence?.is_online) {
      return undefined;
    }

    let cancelled = false;

    async function pollPendingOffer() {
      try {
        const response = await getPendingDispatchOffer();

        if (!cancelled) {
          setPendingOffer(response?.data ?? null);
          setOfferError("");
        }
      } catch (err) {
        if (cancelled) {
          return;
        }

        if (err instanceof ApiError && err.status === 404) {
          setPendingOffer(null);
          setOfferError("");
          return;
        }

        setOfferError(err.message || "Unable to load ride offers.");
      }
    }

    pollPendingOffer();

    const intervalID = window.setInterval(
      pollPendingOffer,
      OFFER_POLL_INTERVAL_MS,
    );

    return () => {
      cancelled = true;
      window.clearInterval(intervalID);
    };
  }, [presence?.is_online]);

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

      setActiveTrip(null);
      setTripError("");
      setPendingOffer(null);

      await loadPresence();
    } catch (err) {
      setError(err.message || "Unable to go offline.");
    } finally {
      setUpdating(false);
    }
  }

  async function handleTripStatusUpdate(status) {
    if (!activeTrip?.id) {
      return;
    }

    setTripUpdating(true);
    setTripError("");

    try {
      await updateTripStatus(activeTrip.id, status);
      await loadActiveTrip();
    } catch (err) {
      setTripError(err.message || "Unable to update trip status.");
    } finally {
      setTripUpdating(false);
    }
  }

  async function handleCompleteTrip() {
    if (!activeTrip?.id) {
      return;
    }

    setTripUpdating(true);
    setTripError("");

    try {
      await completeTrip(activeTrip.id);

      setActiveTrip(null);

      await Promise.all([loadPresence(), loadTripHistory(), loadEarnings()]);
    } catch (err) {
      setTripError(err.message || "Unable to complete trip.");
    } finally {
      setTripUpdating(false);
    }
  }

  async function handleAcceptOffer(offerID) {
    setOfferResponding(true);
    setOfferError("");

    try {
      await acceptDispatchOffer(offerID);
      setPendingOffer(null);

      await Promise.all([loadPresence(), loadActiveTrip()]);
    } catch (err) {
      setOfferError(err.message || "Unable to accept ride offer.");
      await loadPendingOffer();
    } finally {
      setOfferResponding(false);
    }
  }

  async function handleRejectOffer(offerID) {
    const reason = window.prompt(
      "Why are you rejecting this ride?",
      "Driver unavailable",
    );

    if (reason === null) {
      return;
    }

    setOfferResponding(true);
    setOfferError("");

    try {
      await rejectDispatchOffer(offerID, reason.trim());
      setPendingOffer(null);

      await loadPendingOffer();
    } catch (err) {
      setOfferError(err.message || "Unable to reject ride offer.");
      await loadPendingOffer();
    } finally {
      setOfferResponding(false);
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

      <EarningsCard
        dashboard={earnings}
        loading={earningsLoading}
        error={earningsError}
      />

      <TripHistoryCard
        trips={tripHistory}
        loading={tripHistoryLoading}
        error={tripHistoryError}
      />

      {presence?.is_online && (
        <>
          {offerError && <p className="error-message">{offerError}</p>}
          {tripError && <p className="error-message">{tripError}</p>}

          {activeTrip?.status === "IN_PROGRESS" && locationError && (
            <p className="error-message">Trip location: {locationError}</p>
          )}

          <ActiveTripCard
            trip={activeTrip}
            updating={tripUpdating}
            onStatusUpdate={handleTripStatusUpdate}
            onComplete={handleCompleteTrip}
          />

          <DispatchOfferCard
            offer={pendingOffer}
            responding={offerResponding}
            onAccept={handleAcceptOffer}
            onReject={handleRejectOffer}
          />
        </>
      )}
    </section>
  );
}
