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
	tripStreamHandler *TripStreamHandler,
	paymentHandler *PaymentHandler,
	paymentTransactionHandler *PaymentTransactionHandler,
	paymentExecutionHandler *PaymentExecutionHandler,
	paymentCallbackHandler *PaymentCallbackHandler,
	rideRequestHandler *RideRequestHandler,
	serviceCategoryHandler *ServiceCategoryHandler,
	fareEstimateHandler *FareEstimateHandler,
	dispatchHandler *DispatchHandler,
) {

	// Establishes a base versioning group to prevent breaking mobile client contracts during API updates.
	v1 := router.Group("/api/v1")
	{
		// ---------------------------------------------------
		// Public Routes (Accessible without JWT tokens)
		// ---------------------------------------------------

		// ---------------------------------------------------
		// Payment Provider Callback Routes
		// ---------------------------------------------------
		//
		// Provider callbacks do not use CONNECT user JWT
		// authentication. Their trust boundary is the
		// provider-specific cryptographic verifier.
		registerPaymentCallbackRoutes(
			v1,
			paymentCallbackHandler,
		)

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

		// Secures driver operational endpoints from
		// unauthenticated requests and restricts them to
		// authorized driver accounts.
		driver.Use(authMiddleware.RequireAuth())
		driver.Use(rbacMiddleware.RequirePermission("driver.operations"))

		{
			// Tracks shift initialization and termination.
			driver.POST("/online", driverPresenceHandler.GoOnline)
			driver.POST("/offline", driverPresenceHandler.GoOffline)

			// Processes periodic driver telemetry keep-alive.
			driver.POST("/heartbeat", driverPresenceHandler.Heartbeat)

			// Links and releases driver operational assignments.
			driver.POST("/assign", driverAssignmentHandler.Assign)
			driver.POST("/unassign", driverAssignmentHandler.Unassign)

			// Driver availability and presence operations.
			driver.GET("/available", driverPresenceHandler.ListAvailable)
			driver.PATCH("/availability", driverPresenceHandler.UpdateAvailability)

			driver.GET("/presence", driverPresenceHandler.GetCurrent)

			// Recovers the authenticated driver's active trip.
			driver.GET("/trip", tripHandler.GetActiveDriverTrip)
		}

		// ---------------------------------------------------
		// Driver Management Routes (/api/v1/drivers/*)
		// ---------------------------------------------------

		drivers := v1.Group("/drivers")

		drivers.Use(authMiddleware.RequireAuth())

		{
			// ---------------------------------------------------
			// Driver Self-Service Registration
			// ---------------------------------------------------
			//
			// Any authenticated user may submit a driver
			// registration application. Verification and DRIVER
			// role assignment are handled separately.
			drivers.POST(
				"/register",
				driverHandler.Register,
			)

			drivers.GET(
				"/registration",
				driverHandler.GetRegistration,
			)

			drivers.GET(
				"/registration/companies",
				driverHandler.ListRegistrationCompanies,
			)

			drivers.GET(
				"/registration/companies/:company_id/branches",
				driverHandler.ListRegistrationBranches,
			)

			// ---------------------------------------------------
			// Driver Dispatch Offer Operations
			// ---------------------------------------------------
			//
			// Offer ownership and driver authorization are
			// enforced by the dispatch service.
			drivers.GET(
				"/dispatch-offers/pending",
				rbacMiddleware.RequirePermission("driver.operations"),
				dispatchHandler.GetPendingOffer,
			)

			drivers.POST(
				"/dispatch-offers/:offer_id/accept",
				rbacMiddleware.RequirePermission("driver.operations"),
				dispatchHandler.AcceptOffer,
			)

			drivers.POST(
				"/dispatch-offers/:offer_id/reject",
				rbacMiddleware.RequirePermission("driver.operations"),
				dispatchHandler.RejectOffer,
			)

			// ---------------------------------------------------
			// Driver Verification
			// ---------------------------------------------------
			//
			// Driver verification grants operational DRIVER access.
			// This dedicated permission is currently assigned only
			// to SYSTEM_ADMIN.
			drivers.POST(
				"/:id/verify",
				rbacMiddleware.RequirePermission("drivers.verify"),
				driverHandler.Verify,
			)

			// ---------------------------------------------------
			// Administrative Driver Management
			// ---------------------------------------------------
			//
			// Creating, updating, and deleting driver records
			// requires drivers.manage permission. Reading the
			// administrative driver registry requires
			// drivers.read permission.

			drivers.POST(
				"",
				rbacMiddleware.RequirePermission("drivers.manage"),
				driverHandler.Create,
			)

			drivers.GET(
				"",
				rbacMiddleware.RequirePermission("drivers.read"),
				driverHandler.List,
			)

			drivers.GET(
				"/:id",
				rbacMiddleware.RequirePermission("drivers.read"),
				driverHandler.GetByID,
			)

			drivers.PUT(
				"/:id",
				rbacMiddleware.RequirePermission("drivers.manage"),
				driverHandler.Update,
			)

			drivers.DELETE(
				"/:id",
				rbacMiddleware.RequirePermission("drivers.manage"),
				driverHandler.Delete,
			)
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

			trips.GET(
				"/:id/locations",
				tripHandler.ListTripLocations,
			)

			trips.GET(
				"/:id/stream",
				tripStreamHandler.Stream,
			)

			trips.POST(
				"/:id/locations",
				tripHandler.RecordTripLocation,
			)

			trips.POST(
				"/:id/payments",
				paymentHandler.CreateForCompletedTrip,
			)

			trips.GET(
				"/:id/payment",
				paymentHandler.GetTripPayment,
			)
		}
		// ---------------------------------------------------
		// Payment Routes (/api/v1/payments/*)
		// ---------------------------------------------------

		payments := v1.Group("/payments")

		payments.Use(authMiddleware.RequireAuth())

		{
			payments.GET(
				"/:id",
				paymentHandler.GetPayment,
			)

			if paymentTransactionHandler != nil {
				payments.POST(
					"/:id/transactions",
					paymentTransactionHandler.Initiate,
				)
			}
		}

		registerPaymentExecutionRoutes(
			v1,
			authMiddleware,
			paymentExecutionHandler,
		)

		// ---------------------------------------------------
		// Service Category Routes (/api/v1/service-categories)
		// ---------------------------------------------------

		serviceCategories := v1.Group("/service-categories")

		serviceCategories.Use(authMiddleware.RequireAuth())

		{
			serviceCategories.GET(
				"",
				serviceCategoryHandler.ListActive,
			)
		}

		// ---------------------------------------------------
		// Fare Estimate Routes (/api/v1/fare-estimates)
		// ---------------------------------------------------

		fareEstimates := v1.Group("/fare-estimates")

		fareEstimates.Use(authMiddleware.RequireAuth())
		{
			fareEstimates.POST(
				"",
				fareEstimateHandler.Estimate,
			)
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

func registerPaymentCallbackRoutes(
	v1 *gin.RouterGroup,
	handler *PaymentCallbackHandler,
) {
	if handler == nil {
		return
	}

	v1.POST(
		"/payment-callbacks/:provider",
		handler.Handle,
	)
}

func registerPaymentExecutionRoutes(
	v1 *gin.RouterGroup,
	authMiddleware *middleware.AuthMiddleware,
	handler *PaymentExecutionHandler,
) {
	if handler == nil {
		return
	}

	payments :=
		v1.Group("/payments")

	payments.Use(
		authMiddleware.RequireAuth(),
	)

	payments.POST(
		"/:id/transactions/:transaction_id/execute",
		handler.Execute,
	)
}
