function formatMoney(amount, currency) {
    const value = Number(amount);
  
    if (!Number.isFinite(value)) {
      return `${amount ?? "0"} ${currency || "EUR"}`;
    }
  
    return new Intl.NumberFormat("fi-FI", {
      style: "currency",
      currency: currency || "EUR",
    }).format(value);
  }
  
  export function EarningsCard({ dashboard, loading, error }) {
    if (loading) {
      return (
        <div className="driver-status-card">
          <h2>Earnings</h2>
          <p className="muted">Loading earnings...</p>
        </div>
      );
    }
  
    if (error) {
      return (
        <div className="driver-status-card">
          <h2>Earnings</h2>
          <p className="error-message">{error}</p>
        </div>
      );
    }
  
    const summary = dashboard?.summary;
    const recentEarnings = dashboard?.recent_earnings ?? [];
    const currency = summary?.currency || "EUR";
  
    return (
      <div className="driver-status-card">
        <h2>Earnings</h2>
  
        <div className="earnings-summary">
          <div>
            <span className="muted">Net earnings</span>
            <strong>
              {formatMoney(summary?.net_amount ?? "0", currency)}
            </strong>
          </div>
  
          <div>
            <span className="muted">Gross earnings</span>
            <strong>
              {formatMoney(summary?.gross_amount ?? "0", currency)}
            </strong>
          </div>
  
          <div>
            <span className="muted">Completed paid trips</span>
            <strong>{summary?.trip_count ?? 0}</strong>
          </div>
        </div>
  
        <h3>Recent earnings</h3>
  
        {recentEarnings.length === 0 ? (
          <p className="muted">No earnings recorded yet.</p>
        ) : (
          <div className="earnings-list">
            {recentEarnings.map((earning) => (
              <div className="earnings-row" key={earning.id}>
                <div>
                  <strong>
                    {formatMoney(earning.net_amount, earning.currency)}
                  </strong>
                  <span className="muted">
                    Trip {earning.trip_id}
                  </span>
                </div>
  
                <span>{earning.settlement_status}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    );
  }