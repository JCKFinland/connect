package api

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/pkg/logger"
)

func NewRouter(log *slog.Logger, db *pgxpool.Pool) *gin.Engine {

	router := gin.New()

	if err := router.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		log.Error("Failed to configure trusted proxies", "error", err)
	}

	router.Use(gin.Recovery())

	router.Use(logger.Middleware(log))

	RegisterRoutes(router, db)

	return router
}
