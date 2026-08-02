package api

import (
	// Injects Gin to process the health network route and return status payloads.
	"github.com/gin-gonic/gin"
	// Connects directly to the high-performance PostgreSQL connection pool driver.
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

// HealthHandler acts as a functional route generator injected during application bootstrap.
func HealthHandler(db *pgxpool.Pool) gin.HandlerFunc {
	// Returns an anonymous closure function adhering to standard Gin handler signatures.
	return func(c *gin.Context) {

		// Pings the PostgreSQL database pool using internal helper scripts.
		if err := database.Health(db); err != nil {

			// If the database is dead, locked, or unreachable, fail with HTTP 500.
			response.InternalServerError(c)

			return
		}

		// Returns an HTTP 200 OK wrapper containing an explicit health object.
		response.OK(
			c,
			"CONNECT Backend is running",
			gin.H{
				"status": "UP", // Industry standard key indicating a healthy node.
			},
		)
	}
}
