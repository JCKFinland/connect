package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// CONNECT internal architecture imports
	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	driverservice "github.com/JCKFinland/connect/backend/internal/services/driver"

	"github.com/JCKFinland/connect/backend/internal/security"

	authservice "github.com/JCKFinland/connect/backend/internal/services/auth"

	branchservice "github.com/JCKFinland/connect/backend/internal/services/branch"

	companyservice "github.com/JCKFinland/connect/backend/internal/services/company"

	dispatchofferservice "github.com/JCKFinland/connect/backend/internal/services/dispatch_offer"

	fleetservice "github.com/JCKFinland/connect/backend/internal/services/fleet"

	vehicleservice "github.com/JCKFinland/connect/backend/internal/services/vehicle"

	dvassignmentservice "github.com/JCKFinland/connect/backend/internal/services/driver_vehicle_assignment"

	tripservice "github.com/JCKFinland/connect/backend/internal/services/trip"

	rideRequestService "github.com/JCKFinland/connect/backend/internal/services/ride_request"

	assignment "github.com/JCKFinland/connect/backend/internal/services/assignment"

	presence "github.com/JCKFinland/connect/backend/internal/services/presence"

	rbac "github.com/JCKFinland/connect/backend/internal/services/rbac"

	dispatchservice "github.com/JCKFinland/connect/backend/internal/services/dispatch"

	"github.com/JCKFinland/connect/backend/pkg/logger"
)

func main() {

	// ----------------------------------------------------------------------
	// Configuration
	// ----------------------------------------------------------------------

	// Reads environment variables and config files into a Go struct.
	cfg, err := config.Load()
	if err != nil {
		// Hard exit if application settings are corrupt or missing.
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Instantiates structured logging matching the current environment (e.g., JSON for Production).
	log := logger.New(
		cfg.Log.Level,
		cfg.App.Env,
	)

	log.Info("Starting CONNECT Backend")

	// ----------------------------------------------------------------------
	// Database
	// ----------------------------------------------------------------------

	// Establishes the database connection pool using configurations.
	db, err := database.Connect(cfg)
	if err != nil {
		log.Error("Database connection failed", "error", err)
		os.Exit(1)
	}
	// Ensures database connections safely close when the main function terminates.
	defer db.Close()

	// Automatically executes SQL schema migrations to keep database tables updated.
	if err := database.RunMigrations(cfg, log); err != nil {
		log.Error("Migration failed", "error", err)
		os.Exit(1)
	}

	// ----------------------------------------------------------------------
	// Repositories (Data Access Layer)
	// ----------------------------------------------------------------------

	// Mounts core application tables to interface with database queries.
	userRepo := repository.NewUserRepository(db)
	roleRepo := repository.NewRoleRepository(db)
	userRoleRepo := repository.NewUserRoleRepository(db)
	refreshTokenRepo := repository.NewRefreshTokenRepository(db)

	// Mounts taxi-specific operational storage modules mapping to PostgreSQL.
	driverPresenceRepo := postgresrepo.NewDriverPresenceRepository(db)
	driverAssignmentRepo := postgresrepo.NewDriverAssignmentRepository(db)
	companyRepo := postgresrepo.NewCompanyRepository(db)
	branchRepo := postgresrepo.NewBranchRepository(db)
	fleetRepository := postgresrepo.NewFleetRepository(db)
	vehicleRepo := postgresrepo.NewVehicleRepository(db)
	driverRepo := postgresrepo.NewDriverRepository(db)
	driverVehicleAssignmentRepo :=
		postgresrepo.NewDriverVehicleAssignmentRepository(db)
	tripRepo := postgresrepo.NewTripRepository(db)
	rideRequestRepo := postgresrepo.NewRideRequestRepository(db)
	tripEventRepo := postgresrepo.NewTripEventRepository(db)
	dispatchOfferRepo := postgresrepo.NewDispatchOfferRepository(db)

	// ----------------------------------------------------------------------
	// Security
	// ----------------------------------------------------------------------

	// Generates a utility manager for signing and validating JWT tokens.
	jwtService := security.NewJWTService(cfg)

	// ----------------------------------------------------------------------
	// Services (Business Logic Layer)
	// ----------------------------------------------------------------------

	// Injects data tables and JWT tools to form the Authentication logic engine.
	authService := authservice.NewService(
		authservice.Dependencies{
			Config:        cfg,
			Users:         userRepo,
			Roles:         roleRepo,
			UserRoles:     userRoleRepo,
			RefreshTokens: refreshTokenRepo,
			JWT:           jwtService,
		},
	)

	// Encapsulates taxi company onboarding and fleet management processes.
	companyService := companyservice.NewService(
		companyservice.Dependencies{
			Config:    cfg,
			Companies: companyRepo,
		},
	)

	fleetService := fleetservice.NewService(fleetRepository)

	vehicleService := vehicleservice.NewService(vehicleRepo)

	branchService := branchservice.NewService(
		branchservice.Dependencies{
			Config:   cfg,
			Branches: branchRepo,
		},
	)

	driverService := driverservice.NewService(
		driverRepo,
	)

	driverVehicleAssignmentService := dvassignmentservice.NewService(
		driverVehicleAssignmentRepo)

	tripService := tripservice.NewService(
		tripservice.Dependencies{
			DB:           db,
			Trips:        tripRepo,
			RideRequests: rideRequestRepo,
			Presence:     driverPresenceRepo,
			TripEvents:   tripEventRepo,
			UserRoles:    userRoleRepo,
		},
	)

	rideRequestService := rideRequestService.NewService(
		rideRequestService.Dependencies{
			Config:       cfg,
			RideRequests: rideRequestRepo,
			UserRoles:    userRoleRepo,
		},
	)

	dispatchOfferService := dispatchofferservice.NewService(
		dispatchofferservice.Dependencies{
			DB:     db,
			Offers: dispatchOfferRepo,
		},
	)
	_ = dispatchOfferService

	dispatchService := dispatchservice.NewService(
		dispatchservice.Dependencies{
			DB:           db,
			Config:       cfg,
			Logger:       log,
			RideRequests: rideRequestRepo,
			Assignments:  driverAssignmentRepo,
			Presence:     driverPresenceRepo,
			Trips:        tripRepo,
			Vehicles:     vehicleRepo,
			Drivers:      driverRepo,
			Offers:       dispatchOfferRepo,
		},
	)

	// ----------------------------------------------------------------------
	// Automatic Redispatch Worker
	// ----------------------------------------------------------------------

	// Worker context is independent from individual HTTP requests and remains
	// active for the lifetime of the CONNECT backend process.
	redispatchCtx, cancelRedispatch := context.WithCancel(
		context.Background(),
	)

	go dispatchService.StartRedispatchWorker(
		redispatchCtx,
		dispatchservice.RedispatchWorkerOptions{
			Interval:  2 * time.Second,
			BatchSize: 100,
		},
	)

	// Tracks driver shifts, live maps, and online/offline status values.
	presenceService := presence.NewService(
		presence.Dependencies{
			Config:      cfg,
			Users:       userRepo,
			Drivers:     driverRepo,
			Presence:    driverPresenceRepo,
			Assignments: driverAssignmentRepo,
		},
	)

	// Matches trip orders to close by active drivers using presence data.
	assignmentService := assignment.NewService(
		assignment.Dependencies{
			DB:          db,
			Assignments: driverAssignmentRepo,
			Presence:    presenceService,
		},
	)
	// ----------------------------------------------------------------------
	// RBAC Service (Access Control)
	// ----------------------------------------------------------------------

	permissionRepo := repository.NewPermissionRepository(db)

	// Evaluates granular user rights (e.g., checking if a user is an admin, driver, or passenger).
	rbacService := rbac.NewService(permissionRepo)

	// ----------------------------------------------------------------------
	// Middleware (Request Interceptors)
	// ----------------------------------------------------------------------

	// Intercepts inbound calls to validate incoming user tokens.
	authMiddleware := middleware.NewAuthMiddleware(
		jwtService,
		userRepo,
	)

	// Intercepts validated routes to ensure the user has acceptable role rights.
	rbacMiddleware := middleware.NewRBACMiddleware(
		rbacService,
	)

	// ----------------------------------------------------------------------
	// Handlers (API Route Controllers)
	// ----------------------------------------------------------------------

	// Maps standard HTTP endpoints to specific application business logic.
	authHandler := api.NewAuthHandler(authService)
	userHandler := api.NewUserHandler()
	driverPresenceHandler := api.NewDriverPresenceHandler(presenceService)
	companyHandler := api.NewCompanyHandler(companyService)
	branchHandler := api.NewBranchHandler(branchService)
	driverAssignmentHandler := api.NewDriverAssignmentHandler(assignmentService)
	fleetHandler := api.NewFleetHandler(fleetService)
	vehicleHandler := api.NewVehicleHandler(vehicleService)
	driverHandler := api.NewDriverHandler(driverService)
	driverVehicleAssignmentHandler :=
		api.NewDriverVehicleAssignmentHandler(
			driverVehicleAssignmentService,
		)
	tripHandler := api.NewTripHandler(tripService)

	rideRequestHandler := api.NewRideRequestHandler(
		rideRequestService,
	)

	dispatchHandler := api.NewDispatchHandler(
		dispatchService,
	)

	// ----------------------------------------------------------------------
	// Router
	// ----------------------------------------------------------------------

	// Combines logging, endpoints, security middleware, and routes into a uniform network matrix.
	router := api.NewRouter(
		log,
		db,
		authHandler,
		authMiddleware,
		rbacMiddleware,
		userHandler,
		driverPresenceHandler,
		driverAssignmentHandler,
		branchHandler,
		companyHandler,
		fleetHandler,
		vehicleHandler,
		driverHandler,
		driverVehicleAssignmentHandler,
		tripHandler,
		rideRequestHandler,
		dispatchHandler,
	)
	// ----------------------------------------------------------------------
	// HTTP Server Configuration
	// ----------------------------------------------------------------------

	// Instantiates server properties with custom protective connection timeouts.
	server := &http.Server{
		Addr:              ":" + cfg.App.Port,
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Fires off the network listener inside a concurrent background goroutine.
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
	// Graceful Shutdown System
	// ----------------------------------------------------------------------

	// Creates a channel listening for OS-level kills (Ctrl+C, termination commands).
	quit := make(chan os.Signal, 1)

	signal.Notify(
		quit,
		os.Interrupt,
		syscall.SIGTERM,
	)

	// Pauses execution stream here until a cancellation signal arrives.
	<-quit

	log.Info("Shutdown signal received")

	// Stop background dispatch processing before shutting down the HTTP server.
	cancelRedispatch()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	// Forces server down neatly without terminating active passenger/driver operations abruptly.
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
