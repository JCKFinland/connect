package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"

	"github.com/JCKFinland/connect/backend/internal/security"

	authservice "github.com/JCKFinland/connect/backend/internal/services/auth"

	companyservice "github.com/JCKFinland/connect/backend/internal/services/company"

	assignment "github.com/JCKFinland/connect/backend/internal/services/assignment"

	presence "github.com/JCKFinland/connect/backend/internal/services/presence"

	rbac "github.com/JCKFinland/connect/backend/internal/services/rbac"

	"github.com/JCKFinland/connect/backend/pkg/logger"
)

func main() {

	// ----------------------------------------------------------------------
	// Configuration
	// ----------------------------------------------------------------------

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(
		cfg.Log.Level,
		cfg.App.Env,
	)

	log.Info("Starting CONNECT Backend")

	// ----------------------------------------------------------------------
	// Database
	// ----------------------------------------------------------------------

	db, err := database.Connect(cfg)
	if err != nil {
		log.Error("Database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.RunMigrations(cfg, log); err != nil {
		log.Error("Migration failed", "error", err)
		os.Exit(1)
	}

	// ----------------------------------------------------------------------
	// Repositories
	// ----------------------------------------------------------------------

	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	userRoleRepo := repository.NewUserRoleRepository(db)
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)

	driverPresenceRepo := postgresrepo.NewDriverPresenceRepository(db)
	driverAssignmentRepo := postgresrepo.NewDriverAssignmentRepository(db)
	companyRepo := postgresrepo.NewCompanyRepository(db)

	// ----------------------------------------------------------------------
	// Security
	// ----------------------------------------------------------------------

	jwtService := security.NewJWTService(cfg)

	// ----------------------------------------------------------------------
	// Services
	// ----------------------------------------------------------------------

	authService := authservice.NewService(
		authservice.Dependencies{
			Config: cfg,

			Users: userRepo,

			Roles: roleRepo,

			UserRoles: userRoleRepo,

			RefreshTokens: refreshTokenRepo,

			JWT: jwtService,
		},
	)

	companyService := companyservice.NewService(
		companyservice.Dependencies{
			Config: cfg,
			Companies: companyRepo,
		},
	)

	presenceService := presence.NewService(
		presence.Dependencies{
			Config: cfg,

			Users: userRepo,

			Presence: driverPresenceRepo,

			Assignments: driverAssignmentRepo,
		},
	)

	assignmentService := assignment.NewService(
		assignment.Dependencies{
			Assignments: driverAssignmentRepo,
			Presence:    presenceService,
		},
	)

	// ----------------------------------------------------------------------
	// RBAC Service
	// ----------------------------------------------------------------------

	permissionRepo := repository.NewPermissionRepository(db)

	rbacService := rbac.NewService(permissionRepo)

	// ----------------------------------------------------------------------
	// Middleware
	// ----------------------------------------------------------------------

	authMiddleware := middleware.NewAuthMiddleware(
		jwtService,
		userRepo,
	)

	rbacMiddleware := middleware.NewRBACMiddleware(
		rbacService,
	)

	// ----------------------------------------------------------------------
	// Handlers
	// ----------------------------------------------------------------------

	authHandler := api.NewAuthHandler(authService)
	userHandler := api.NewUserHandler()
	driverPresenceHandler := api.NewDriverPresenceHandler(
		presenceService,
	)

	companyHandler := api.NewCompanyHandler(
		companyService,
	)

	driverAssignmentHandler := api.NewDriverAssignmentHandler(
		assignmentService,
	)

	// ----------------------------------------------------------------------
	// Router
	// ----------------------------------------------------------------------

	router := api.NewRouter(
		log,
		db,
		authHandler,
		authMiddleware,
		rbacMiddleware,
		userHandler,
		driverPresenceHandler,
		driverAssignmentHandler,
		companyHandler,
	)

	// ----------------------------------------------------------------------
	// HTTP Server
	// ----------------------------------------------------------------------

	server := &http.Server{
		Addr:              ":" + cfg.App.Port,
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {

		log.Info(
			"HTTP server started",
			"address",
			server.Addr,
		)

		if err := server.ListenAndServe(); err != nil &&
			err != http.ErrServerClosed {

			log.Error(
				"HTTP server failed",
				"error",
				err,
			)

			os.Exit(1)
		}
	}()

	// ----------------------------------------------------------------------
	// Graceful Shutdown
	// ----------------------------------------------------------------------

	quit := make(chan os.Signal, 1)

	signal.Notify(
		quit,
		os.Interrupt,
		syscall.SIGTERM,
	)

	<-quit

	log.Info("Shutdown signal received")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {

		log.Error(
			"Graceful shutdown failed",
			"error",
			err,
		)

		os.Exit(1)
	}

	log.Info("CONNECT Backend stopped")
}
