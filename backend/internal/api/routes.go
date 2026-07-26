package api

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RegisterRoutes(router *gin.Engine, db *pgxpool.Pool) {

	v1 := router.Group("/api/v1")
	{
		v1.GET("/health", HealthHandler(db))
	}
}