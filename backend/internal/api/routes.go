package api

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/middleware"
)

func RegisterRoutes(
	router *gin.Engine,
	db *pgxpool.Pool,
	authHandler *AuthHandler,
	authMiddleware *middleware.AuthMiddleware,
	userHandler *UserHandler,
) {

	v1 := router.Group("/api/v1")
	{

		// Public routes
		v1.GET("/health", HealthHandler(db))

		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/refresh", authHandler.Refresh)
			authGroup.POST("/logout", authHandler.Logout)
		}

		// Protected routes
		users := v1.Group("/users")
		users.Use(authMiddleware.RequireAuth())
		{
			users.GET("/me", userHandler.Me)
		}
	}
}
