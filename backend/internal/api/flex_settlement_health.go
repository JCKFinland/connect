package api

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

const flexSettlementHealthStaleAfter = 15 * time.Minute

// FlexSettlementHealthHandler exposes aggregate settlement operational
// health. Authentication and authorization must be enforced by the route.
//
// This handler never executes captures, reconciliation, or settlement
// state transitions.
func FlexSettlementHealthHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot, err := postgres.ReadFlexSettlementHealthSnapshot(
			c.Request.Context(),
			db,
			flexSettlementHealthStaleAfter,
		)
		if err != nil {
			response.InternalServerError(c)
			return
		}

		response.OK(c, "FLEX settlement health snapshot", gin.H{
			"observed_at":                 snapshot.ObservedAt,
			"stale_after_seconds":         int64(flexSettlementHealthStaleAfter.Seconds()),
			"pending_capture_count":       snapshot.PendingCaptureCount,
			"recovery_count":              snapshot.RecoveryCount,
			"stale_pending_capture_count": snapshot.StalePendingCaptureCount,
			"stale_recovery_count":        snapshot.StaleRecoveryCount,
			"oldest_pending_capture_at":   snapshot.OldestPendingCaptureAt,
			"oldest_recovery_at":          snapshot.OldestRecoveryAt,
		})
	}
}
