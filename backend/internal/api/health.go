package api

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

func HealthHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {

		if err := database.Health(db); err != nil {

			response.InternalServerError(
				c,
				"Database unavailable",
			)

			return
		}

		response.OK(
			c,
			"CONNECT Backend is running",
			gin.H{
				"status": "UP",
			},
		)
	}
}