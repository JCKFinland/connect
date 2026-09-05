package api

import (
	// Injects Gin to orchestrate endpoint groups, URL paths, and middleware integration.
	"github.com/gin-gonic/gin"
	// Passes the raw PostgreSQL connection pool down to operational health checks.
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
	driverPresenceHandler *DriverPresenceHandler,
	driverAssignmentHandler *DriverAssignmentHandler,
	branchHandler *BranchHandler,
	companyHandler *CompanyHandler,
	fleetHandler *FleetHandler,
	vehicleHandler *VehicleHandler,
	driverHandler *DriverHandler,
	driverVehicleAssignmentHandler *DriverVehicleAssignmentHandler,
	tripHandler *TripHandler,
	paymentHandler *PaymentHandler,
	rideRequestHandler *RideRequestHandler,
	dispatchHandler *DispatchHandler,
) {

	// Establishes a base versioning group to prevent breaking mobile client contracts during API updates.
	v1 := router.Group("/api/v1")
	{
		// ---------------------------------------------------
		// Public Routes (Accessible without JWT tokens)
		// ---------------------------------------------------

		// Attaches the explicit closure checking database connectivity metrics.
		v1.GET("/health", HealthHandler(db))

		// Bundles authentication requests into a semantic sub-group (/api/v1/auth/*).
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
			auth.POST("/logout", authHandler.Logout)
		}

		// ---------------------------------------------------
		// Protected Routes (Require valid Session Token checks)
		// ---------------------------------------------------

		users := v1.Group("/users")

		// Intercepts user requests to validate the caller's identity via JWT verification.
		users.Use(authMiddleware.RequireAuth())
		{
			users.GET("/me", userHandler.Me)
		}

		// ---------------------------------------------------
		// Example RBAC (Requires Identity + Specific Permission verification)
		// ---------------------------------------------------

		admin := v1.Group("/admin")

		// Enforces double-layer security: must be logged in AND have "users.read" rights.
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

		// ---------------------------------------------------
		// Driver Operations Routing Group (/api/v1/driver/*)
		// ---------------------------------------------------

		driver := v1.Group("/driver")

		// Secures the driver dispatch system from unauthenticated requests.
		driver.Use(authMiddleware.RequireAuth())
		{
			// Tracks shift initialization and termination sequences.
			driver.POST("/online", driverPresenceHandler.GoOnline)
			driver.POST("/offline", driverPresenceHandler.GoOffline)

			// Processes periodic telemetry keep-alive updates from the active mobile map.
			driver.POST("/heartbeat", driverPresenceHandler.Heartbeat)

			// Links and cuts driver vehicle/dispatch alignments.
			driver.POST("/assign", driverAssignmentHandler.Assign)
			driver.POST("/unassign", driverAssignmentHandler.Unassign)

			// Updates operational statuses (e.g., changing from "Available" to "On Break").
			driver.GET("/available", driverPresenceHandler.ListAvailable)
			driver.PATCH("/availability", driverPresenceHandler.UpdateAvailability)

		}

		// ---------------------------------------------------
		// Company Routes REST CRUD Group (/api/v1/companies/*)
		// ---------------------------------------------------

		companies := v1.Group("/companies")

		// Mandates valid system login credentials to alter or view fleet corporate metadata.
		companies.Use(authMiddleware.RequireAuth())
		{
			// Standard REST patterns mapping to Create, List, Read, Update, and Delete operations.
			companies.POST("", companyHandler.Create)
			companies.GET("", companyHandler.List)
			companies.GET("/:id", companyHandler.GetByID) // Captures specific string keys dynamically.
			companies.PUT("/:id", companyHandler.Update)
			companies.DELETE("/:id", companyHandler.Delete)
		}

		branches := v1.Group("/branches")

		branches.Use(authMiddleware.RequireAuth())

		{
			branches.POST("", branchHandler.Create)
			branches.GET("", branchHandler.List)
			branches.GET("/:id", branchHandler.GetByID)
			branches.PUT("/:id", branchHandler.Update)
			branches.DELETE("/:id", branchHandler.Delete)
		}

		fleets := v1.Group("/fleets")

		fleets.Use(authMiddleware.RequireAuth())

		{
			fleets.POST("", fleetHandler.Create)

			fleets.GET("", fleetHandler.List)

			fleets.GET("/:id", fleetHandler.GetByID)

			fleets.PUT("/:id", fleetHandler.Update)

			fleets.DELETE("/:id", fleetHandler.Delete)
		}

		vehicles := v1.Group("/vehicles")

		vehicles.Use(authMiddleware.RequireAuth())

		{
			vehicles.POST("", vehicleHandler.Create)

			vehicles.GET("", vehicleHandler.List)

			vehicles.GET("/:id", vehicleHandler.GetByID)

			vehicles.PUT("/:id", vehicleHandler.Update)

			vehicles.DELETE("/:id", vehicleHandler.Delete)
		}

		drivers := v1.Group("/drivers")

		drivers.Use(authMiddleware.RequireAuth())

		{
			drivers.POST("", driverHandler.Create)

			drivers.GET("", driverHandler.List)

			drivers.GET("/:id", driverHandler.GetByID)

			drivers.PUT("/:id", driverHandler.Update)

			drivers.DELETE("/:id", driverHandler.Delete)

			drivers.POST(
				"/dispatch-offers/:offer_id/accept",
				dispatchHandler.AcceptOffer,
			)
			drivers.POST(
				"/dispatch-offers/:offer_id/reject",
				dispatchHandler.RejectOffer,
			)
		}

		assignments := v1.Group("/driver-vehicle-assignments")

		assignments.Use(authMiddleware.RequireAuth())

		{
			assignments.POST("", driverVehicleAssignmentHandler.Assign)

			assignments.GET("", driverVehicleAssignmentHandler.List)

			assignments.GET("/:id", driverVehicleAssignmentHandler.GetByID)

			assignments.PATCH("/:id/release", driverVehicleAssignmentHandler.Release)

			assignments.DELETE("/:id", driverVehicleAssignmentHandler.Delete)
		}

		// ---------------------------------------------------
		// Trip Management Routes (/api/v1/trips/*)
		// ---------------------------------------------------

		trips := v1.Group("/trips")

		trips.Use(authMiddleware.RequireAuth())

		{
			trips.POST("", tripHandler.CreateTrip)
			trips.GET("", tripHandler.ListTrips)
			trips.GET("/:id", tripHandler.GetTrip)
			trips.PUT("/:id", tripHandler.UpdateTrip)
			trips.DELETE("/:id", tripHandler.DeleteTrip)

			trips.PATCH("/:id/status", tripHandler.UpdateTripStatus)
			trips.POST("/:id/complete", tripHandler.Complete)
			trips.POST("/:id/assign", tripHandler.AssignDriver)
			trips.GET("/:id/events", tripHandler.ListTripEvents)
			trips.POST("/:id/locations", tripHandler.RecordTripLocation)

			trips.POST("/:id/payments", paymentHandler.CreateForCompletedTrip)
			trips.GET("/:id/payment", paymentHandler.GetTripPayment)
		}

		// ---------------------------------------------------
		// Payment Routes (/api/v1/payments/*)
		// ---------------------------------------------------

		payments := v1.Group("/payments")

		payments.Use(authMiddleware.RequireAuth())

		{
			payments.GET("/:id", paymentHandler.GetPayment)
		}

		// ---------------------------------------------------
		// Ride Request Routes (/api/v1/ride-requests/*)
		// ---------------------------------------------------

		rideRequests := v1.Group("/ride-requests")

		rideRequests.Use(authMiddleware.RequireAuth())

		{
			rideRequests.POST(
				"",
				rideRequestHandler.Create,
			)

			rideRequests.GET(
				"",
				rideRequestHandler.List,
			)

			rideRequests.GET(
				"/:id",
				rideRequestHandler.GetByID,
			)

			rideRequests.PUT(
				"/:id",
				rideRequestHandler.Update,
			)

			rideRequests.PATCH(
				"/:id/status",
				rideRequestHandler.UpdateStatus,
			)

			rideRequests.POST(
				"/:id/dispatch",
				rbacMiddleware.RequirePermission("rides.dispatch"),
				dispatchHandler.DispatchRide,
			)
		}
	}
}
