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
	rbacMiddleware *middleware.RBACMiddleware,
	userHandler *UserHandler,
) {

	v1 := router.Group("/api/v1")
	{

		// ---------------------------------------------------
		// Public Routes
		// ---------------------------------------------------

		v1.GET("/health", HealthHandler(db))

		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
			auth.POST("/logout", authHandler.Logout)
		}

		// ---------------------------------------------------
		// Protected Routes
		// ---------------------------------------------------

		users := v1.Group("/users")

		users.Use(authMiddleware.RequireAuth())

		{
			users.GET("/me", userHandler.Me)
		}

		// ---------------------------------------------------
		// Example RBAC
		// ---------------------------------------------------

		admin := v1.Group("/admin")

		admin.Use(authMiddleware.RequireAuth())
		admin.Use(rbacMiddleware.RequirePermission("users.read"))

		{
			admin.GET("/ping", func(c *gin.Context) {

				c.JSON(200, gin.H{
					"success": true,
					"message": "RBAC working",
				})

			})
		}
	}
}
