package api

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
)

func NewRouter(
	log *slog.Logger,
	db *pgxpool.Pool,
	authHandler *AuthHandler,
	jwtService *security.JWTService,
	userRepo repository.UserRepository,
) *gin.Engine {

	router := gin.New()

	if err := router.SetTrustedProxies(
		[]string{"127.0.0.1", "::1"},
	); err != nil {

		log.Error(
			"failed to configure trusted proxies",
			"error",
			err,
		)
	}

	router.Use(middleware.RequestID())
	router.Use(middleware.Recovery(log))
	router.Use(middleware.CORS())
	router.Use(middleware.Logger(log))

	// Authentication middleware
	authMiddleware := middleware.NewAuthMiddleware(
		jwtService,
		userRepo,
	)

	userHandler := NewUserHandler()

	RegisterRoutes(
		router,
		db,
		authHandler,
		authMiddleware,
		userHandler,
	)

	return router
}
