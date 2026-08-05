package api

import (
	// Injects Go's native high-performance structured logging library.
	"log/slog"

	// Imports the Gin web engine components.
	"github.com/gin-gonic/gin"
	// Connects to the primary PostgreSQL connection pool driver.
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/middleware"
)

// NewRouter serves as the master wiring constructor called during application initialization in main.go.
func NewRouter(
	log *slog.Logger,
	db *pgxpool.Pool,
	auth *AuthHandler,
	authMiddleware *middleware.AuthMiddleware,
	rbacMiddleware *middleware.RBACMiddleware,
	userHandler *UserHandler,
	driverPresenceHandler *DriverPresenceHandler,
	driverAssignmentHandler *DriverAssignmentHandler,
	branchHandler *BranchHandler,
	companyHandler *CompanyHandler,
	fleetHandler *FleetHandler,
	vehicleHandler *VehicleHandler,
	driverHandler *DriverHandler,
) *gin.Engine {

	// Instantiates a blank Gin engine instance without default middleware (like default logger/recovery).
	router := gin.New()

	// Secures the application by restricting proxy header evaluation to local networks.
	if err := router.SetTrustedProxies(
		[]string{"127.0.0.1", "::1"},
	); err != nil {

		log.Error(
			"failed to configure trusted proxies",
			"error",
			err,
		)
	}

	// Injects a globally unique tracing identifier into every incoming HTTP request header.
	router.Use(middleware.RequestID())

	// Prevents the application from crashing globally if a single request encounters a severe panic.
	router.Use(middleware.Recovery(log))

	// Appends Cross-Origin Resource Sharing rules to allow safe browser/web dashboard interaction.
	router.Use(middleware.CORS())

	// Enforces structured logging output for tracking requested paths, latencies, and response codes.
	router.Use(middleware.Logger(log))

	// Feeds the configured engine and structural dependencies straight into the route registry mapper.
	RegisterRoutes(
		router,
		db,
		auth,
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
	)

	// Returns the ready-to-run HTTP network multiplexer pool back to main.go.
	return router
}
