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

		authorizationSnapshot, err := postgres.ReadFlexAuthorizationHealthSnapshot(
			c.Request.Context(),
			db,
			flexSettlementHealthStaleAfter,
		)
		if err != nil {
			response.InternalServerError(c)
			return
		}

		response.OK(c, "FLEX settlement health snapshot", gin.H{
			"authorization_health": gin.H{
				"observed_at": authorizationSnapshot.ObservedAt,
				"stale_after_seconds": int64(
					flexSettlementHealthStaleAfter.Seconds(),
				),
				"pending_count":                           authorizationSnapshot.PendingCount,
				"processing_without_provider_count":       authorizationSnapshot.ProcessingWithoutProviderCount,
				"processing_with_provider_count":          authorizationSnapshot.ProcessingWithProviderCount,
				"authorized_count":                        authorizationSnapshot.AuthorizedCount,
				"stale_pending_count":                     authorizationSnapshot.StalePendingCount,
				"stale_processing_without_provider_count": authorizationSnapshot.StaleProcessingWithoutProviderCount,
				"stale_processing_with_provider_count":    authorizationSnapshot.StaleProcessingWithProviderCount,
				"oldest_pending_at":                       authorizationSnapshot.OldestPendingAt,
				"oldest_processing_without_provider_at":   authorizationSnapshot.OldestProcessingWithoutProviderAt,
				"oldest_processing_with_provider_at":      authorizationSnapshot.OldestProcessingWithProviderAt,
			},
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
