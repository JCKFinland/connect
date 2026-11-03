package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	// CONNECT internal architecture imports
	"github.com/JCKFinland/connect/backend/internal/api"
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/database"
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/realtime"
	"github.com/JCKFinland/connect/backend/internal/repository"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/JCKFinland/connect/backend/internal/security"
	driverservice "github.com/JCKFinland/connect/backend/internal/services/driver"
	drivercomplianceservice "github.com/JCKFinland/connect/backend/internal/services/driver_compliance"
	driverdocumentservice "github.com/JCKFinland/connect/backend/internal/services/driver_document"
	driverearningservice "github.com/JCKFinland/connect/backend/internal/services/driver_earning"
	fareservice "github.com/JCKFinland/connect/backend/internal/services/fare"
	fareestimateservice "github.com/JCKFinland/connect/backend/internal/services/fare_estimate"
	pricingservice "github.com/JCKFinland/connect/backend/internal/services/pricing"
	routingservice "github.com/JCKFinland/connect/backend/internal/services/routing"
	documentstorage "github.com/JCKFinland/connect/backend/internal/storage/document"

	authservice "github.com/JCKFinland/connect/backend/internal/services/auth"

	branchservice "github.com/JCKFinland/connect/backend/internal/services/branch"

	companyservice "github.com/JCKFinland/connect/backend/internal/services/company"

	dispatchofferservice "github.com/JCKFinland/connect/backend/internal/services/dispatch_offer"
	fleetservice "github.com/JCKFinland/connect/backend/internal/services/fleet"

	vehicleservice "github.com/JCKFinland/connect/backend/internal/services/vehicle"

	paymentservice "github.com/JCKFinland/connect/backend/internal/services/payment"

	tripservice "github.com/JCKFinland/connect/backend/internal/services/trip"

	rideRequestService "github.com/JCKFinland/connect/backend/internal/services/ride_request"

	assignment "github.com/JCKFinland/connect/backend/internal/services/assignment"

	presence "github.com/JCKFinland/connect/backend/internal/services/presence"

	rbac "github.com/JCKFinland/connect/backend/internal/services/rbac"

	dispatchservice "github.com/JCKFinland/connect/backend/internal/services/dispatch"

	"github.com/JCKFinland/connect/backend/pkg/logger"

	stripepayment "github.com/JCKFinland/connect/backend/internal/payments/stripe"
	paymentcallbackservice "github.com/JCKFinland/connect/backend/internal/services/paymentcallback"
	paymentexecutionservice "github.com/JCKFinland/connect/backend/internal/services/paymentexecution"
	paymenttransactionservice "github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

func main() {
	os.Exit(run())
}

func run() (exitCode int) {

	// ----------------------------------------------------------------------
	// Configuration
	// ----------------------------------------------------------------------

	// Reads environment variables and config files into a Go struct.
	cfg, err := config.Load()
	if err != nil {
		// Hard exit if application settings are corrupt or missing.
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		return 1
	}

	// Reject incomplete FLEX Stripe configuration before initializing
	// application services or opening the HTTP listener.
	if err := config.ValidateStripeFlexConfiguration(cfg.Stripe); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"FLEX Stripe configuration error: %v\n",
			err,
		)
		return 1
	}

	// Reject invalid Stripe Connect provisioning configuration before
	// initializing the database or opening the HTTP listener.
	// This does not enable provisioning or register any route.
	if err := config.ValidateStripeConnectProvisioningStartup(
		cfg.StripeConnectProvisioning,
		cfg.Stripe,
	); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Stripe Connect provisioning configuration error: %v\n",
			err,
		)
		return 1
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
		return 1
	}
	// Ensures database connections safely close when the main function terminates.
	defer db.Close()

	// Automatically executes SQL schema migrations to keep database tables updated.
	if err := database.RunMigrations(cfg, log); err != nil {
		log.Error("Migration failed", "error", err)
		return 1
	}

	// Verify durable Stripe provisioning storage before constructing
	// repositories or exposing HTTP routes. Disabled provisioning
	// requires no provisioning schema and performs no readiness query.
	provisioningEnabled, err := cfg.StripeConnectProvisioning.Enabled()
	if err != nil {
		log.Error("Stripe provisioning feature gate invalid", "error", err)
		return 1
	}
	if provisioningEnabled {
		if err := database.VerifyStripeProvisioningSchema(
			context.Background(), db,
		); err != nil {
			log.Error("Stripe provisioning schema not ready", "error", err)
			return 1
		}
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
	companyMembershipRepo := postgresrepo.NewCompanyMembershipRepository(db)
	vehicleRepo := postgresrepo.NewVehicleRepository(db)
	driverRepo := postgresrepo.NewDriverRepository(db)
	driverDocumentRepo := postgresrepo.NewDriverDocumentRepository(db)
	tripRepo := postgresrepo.NewTripRepository(db)
	rideRequestRepo := postgresrepo.NewRideRequestRepository(db)
	serviceCategoryRepo := postgresrepo.NewServiceCategoryRepository(db)
	farePricingProfileRepo := postgresrepo.NewFarePricingProfileRepository(db)
	tripEventRepo := postgresrepo.NewTripEventRepository(db)
	dispatchOfferRepo := postgresrepo.NewDispatchOfferRepository(db)
	tripLocationRepo := postgresrepo.NewTripLocationRepository(db)
	paymentRepo := postgresrepo.NewPaymentRepository(db)
	driverEarningRepo :=
		postgresrepo.NewDriverEarningRepository(db)
	paymentTransactionRepo :=
		postgresrepo.NewPaymentTransactionRepository(db)

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
			Config:             cfg,
			DB:                 db,
			Companies:          companyRepo,
			UserRoles:          userRoleRepo,
			CompanyMemberships: companyMembershipRepo,
		},
	)

	fleetService := fleetservice.NewService(
		fleetservice.Dependencies{
			DB:                 db,
			Fleets:             fleetRepository,
			Drivers:            driverRepo,
			Branches:           branchRepo,
			UserRoles:          userRoleRepo,
			CompanyMemberships: companyMembershipRepo,
		},
	)

	vehicleService := vehicleservice.NewService(
		vehicleservice.Dependencies{
			DB:                 db,
			Vehicles:           vehicleRepo,
			Drivers:            driverRepo,
			Fleets:             fleetRepository,
			UserRoles:          userRoleRepo,
			CompanyMemberships: companyMembershipRepo,
		},
	)

	branchService := branchservice.NewService(
		branchservice.Dependencies{
			Config:             cfg,
			DB:                 db,
			Branches:           branchRepo,
			UserRoles:          userRoleRepo,
			CompanyMemberships: companyMembershipRepo,
		},
	)

	driverService := driverservice.NewService(
		driverservice.Dependencies{
			DB:                 db,
			Drivers:            driverRepo,
			Companies:          companyRepo,
			Branches:           branchRepo,
			UserRoles:          userRoleRepo,
			CompanyMemberships: companyMembershipRepo,
		},
	)

	// Driver regulatory-document binaries are intentionally stored outside
	// PostgreSQL. Fail startup if the configured storage implementation cannot
	// be initialized so CONNECT never accepts uploads without durable binary
	// storage.
	var driverDocumentStorage documentstorage.Storage

	switch strings.ToLower(
		strings.TrimSpace(cfg.DocumentStorage.Driver),
	) {
	case "local":
		localDocumentStorage, err :=
			documentstorage.NewLocalStorage(
				cfg.DocumentStorage.LocalRoot,
			)
		if err != nil {
			log.Error(
				"failed to configure driver document storage",
				"driver",
				cfg.DocumentStorage.Driver,
				"error",
				err,
			)
			return 1
		}

		driverDocumentStorage = localDocumentStorage

	default:
		log.Error(
			"unsupported driver document storage driver",
			"driver",
			cfg.DocumentStorage.Driver,
		)
		return 1
	}

	driverComplianceService := drivercomplianceservice.NewService(
		drivercomplianceservice.Dependencies{
			Drivers:   driverRepo,
			Documents: driverDocumentRepo,
		},
	)

	driverDocumentService := driverdocumentservice.NewService(
		driverdocumentservice.Dependencies{
			DB:             db,
			Documents:      driverDocumentRepo,
			Drivers:        driverRepo,
			Storage:        driverDocumentStorage,
			UploadMaxBytes: cfg.DocumentStorage.UploadMaxBytes,
		},
	)

	driverEarningService :=
		driverearningservice.NewService(
			driverearningservice.Dependencies{
				Earnings: driverEarningRepo,
			},
		)

	routingService := routingservice.NewOSRMService(
		cfg.Routing.BaseURL,
		nil,
	)

	pricingService := pricingservice.NewService(
		pricingservice.Dependencies{
			FarePricingProfiles: farePricingProfileRepo,
		},
	)

	fareEstimateService :=
		fareestimateservice.NewService(
			fareestimateservice.Dependencies{
				Routing: routingService,
				Pricing: pricingService,
				Fare:    fareservice.NewService(),

				BookingCompanyID: cfg.Booking.CompanyID,
			},
		)

	rideRequestService := rideRequestService.NewService(
		rideRequestService.Dependencies{
			DB:                db,
			Config:            cfg,
			RideRequests:      rideRequestRepo,
			UserRoles:         userRoleRepo,
			ServiceCategories: serviceCategoryRepo,
			FareEstimator:     fareEstimateService,
		},
	)

	tripService := tripservice.NewService(
		tripservice.Dependencies{
			DB:                 db,
			CompanyMemberships: companyMembershipRepo,
			Trips:              tripRepo,
			RideRequests:       rideRequestRepo,
			RideRequestAccess:  rideRequestService,
			Presence:           driverPresenceRepo,
			TripEvents:         tripEventRepo,
			UserRoles:          userRoleRepo,
			TripLocations:      tripLocationRepo,
			FareCalculator:     fareservice.NewService(),
		},
	)

	paymentService := paymentservice.NewService(
		paymentservice.Dependencies{
			DB:        db,
			Payments:  paymentRepo,
			Trips:     tripRepo,
			UserRoles: userRoleRepo,
		},
	)

	paymentTransactionService :=
		paymenttransactionservice.NewService(db)

	paymentTransactionHandler :=
		api.NewPaymentTransactionHandler(
			paymentService,
			paymentTransactionService,
		)

	var paymentExecutionHandler *api.PaymentExecutionHandler

	if cfg.Stripe.SecretKey != "" {
		stripeExecutor, err :=
			stripepayment.NewExecutor(
				cfg.Stripe.SecretKey,
			)
		if err != nil {
			log.Error(
				"failed to configure Stripe payment executor",
				"error",
				err,
			)
			return 1
		}

		paymentExecutionService :=
			paymentexecutionservice.NewService(
				paymentexecutionservice.Dependencies{
					Transactions: paymentTransactionRepo,

					PaymentTransactions: paymentTransactionService,

					Stripe: stripeExecutor,
				},
			)

		paymentExecutionHandler =
			api.NewPaymentExecutionHandler(
				paymentService,
				paymentTransactionRepo,
				paymentExecutionService,
			)

		log.Info(
			"Stripe payment execution enabled",
		)
	}

	paymentCallbackService :=
		paymentcallbackservice.NewService(
			paymentcallbackservice.Dependencies{
				Transactions: paymentTransactionRepo,

				PaymentTransactions: paymentTransactionService,
			},
		)

	paymentCallbackRegistry :=
		paymentcallbackservice.NewVerifierRegistry()

	var paymentCallbackHandler *api.PaymentCallbackHandler

	if cfg.Stripe.WebhookSecret != "" {
		stripeVerifier, err :=
			stripepayment.NewWebhookVerifier(
				cfg.Stripe.WebhookSecret,
			)
		if err != nil {
			log.Error(
				"failed to configure Stripe webhook verifier",
				"error",
				err,
			)
			return 1
		}

		if err := paymentCallbackRegistry.Register(
			stripepayment.ProviderName,
			stripeVerifier,
		); err != nil {
			log.Error(
				"failed to register Stripe webhook verifier",
				"error",
				err,
			)
			return 1
		}

		paymentCallbackHandler =
			api.NewPaymentCallbackHandler(
				paymentCallbackService,
				paymentCallbackRegistry,
			)

		log.Info(
			"Stripe payment callbacks enabled",
		)
	}

	dispatchOfferService := dispatchofferservice.NewService(
		dispatchofferservice.Dependencies{
			DB:     db,
			Offers: dispatchOfferRepo,
		},
	)
	_ = dispatchOfferService

	// ----------------------------------------------------------------------
	// Stripe FLEX Authorization Webhook
	// ----------------------------------------------------------------------

	flexWebhookHandler, err := buildFlexWebhookHandler(
		db,
		cfg.Stripe,
	)
	if err != nil {
		log.Error(
			"failed to initialize Stripe FLEX webhook",
			"error",
			err,
		)
		return 1
	}

	if flexWebhookHandler != nil {
		log.Info("Stripe FLEX authorization webhook enabled")
	}

	// Passenger authorization is enabled only when the FLEX
	// webhook reconciliation handler initialized successfully.
	passengerAuthorizationHandler, err :=
		buildPassengerFlexAuthorizationHandler(
			db,
			cfg.Stripe,
			flexWebhookHandler,
		)
	if err != nil {
		log.Error(
			"failed to initialize passenger FLEX authorization",
			"error",
			err,
		)
		return 1
	}

	if passengerAuthorizationHandler != nil {
		log.Info("Passenger FLEX authorization endpoint enabled")
	}

	// Passenger confirmation is available only when the
	// authorization and webhook reconciliation pathway is enabled.
	passengerConfirmationHandler, err :=
		buildPassengerFlexConfirmationHandler(
			db,
			cfg,
			passengerAuthorizationHandler,
		)
	if err != nil {
		log.Error(
			"failed to initialize passenger FLEX confirmation",
			"error",
			err,
		)
		return 1
	}

	if passengerConfirmationHandler != nil {
		passengerAuthorizationHandler.SetConfirmationHandler(
			passengerConfirmationHandler,
		)
		log.Info("Passenger FLEX confirmation endpoint enabled")
	}

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
	// FLEX settlement discovery lifecycle startup
	// ----------------------------------------------------------------------
	// Financial processing remains disabled unless explicitly enabled.
	flexWorkerEnabled, err := cfg.FlexSettlementWorker.Enabled()
	if err != nil {
		log.Error(
			"invalid FLEX settlement worker activation",
			"error",
			err,
		)
		return 1
	}

	flexWorkerFailure := make(chan error, 1)
	var recoveryWorkerFailure chan error
	serverFailure := make(chan error, 1)
	var flexController *flexSettlementController

	if flexWorkerEnabled {
		flexDependencies, err := newFlexSettlementWorkerDependencies(
			cfg.Stripe.SecretKey,
		)
		if err != nil {
			log.Error(
				"failed to initialize FLEX settlement dependencies",
				"error",
				err,
			)
			return 1
		}

		flexLifecycle, err := buildFlexSettlementWorker(
			db,
			flexDependencies,
			50,
		)
		if err != nil {
			log.Error(
				"failed to build FLEX settlement worker",
				"error",
				err,
			)
			return 1
		}

		flexController, err = newFlexSettlementController(
			cfg,
			flexLifecycle,
		)
		if err != nil {
			log.Error(
				"failed to configure FLEX settlement controller",
				"error",
				err,
			)
			return 1
		}

		// Protect startup failures after a financial worker has
		// begun. Deferred cleanup runs before PostgreSQL closes.
		defer func() {
			if err := waitForFlexShutdown(
				flexController,
				30*time.Second,
			); err != nil {
				log.Error(
					"FLEX cleanup encountered an error",
					"error",
					err,
				)
				exitCode = 1
			}
		}()

		if err := flexController.Start(
			context.Background(),
			flexSettlementWorkerOptions(
				func(err error) {
					log.Error(
						"FLEX settlement discovery error",
						"error",
						err,
					)
				},
			),
			func(err error) {
				log.Error(
					"FLEX settlement worker exited unexpectedly",
					"error",
					err,
				)

				select {
				case flexWorkerFailure <- err:
				default:
				}
			},
		); err != nil {
			log.Error(
				"failed to start FLEX settlement worker",
				"error",
				err,
			)
			return 1
		}

		log.Info("FLEX settlement discovery worker enabled")
	} else {
		log.Info("FLEX settlement discovery worker disabled")
	}

	// ----------------------------------------------------------------------
	// FLEX authorization recovery lifecycle startup
	// ----------------------------------------------------------------------
	// Recovery is independent from settlement and disabled by default.
	recoveryWorkerEnabled, err :=
		cfg.FlexAuthorizationRecoveryWorker.Enabled()
	if err != nil {
		log.Error(
			"invalid FLEX authorization recovery worker activation",
			"error",
			err,
		)
		return 1
	}

	var recoveryController *flexAuthorizationRecoveryController

	if recoveryWorkerEnabled {
		recoveryWorkerFailure = make(chan error, 1)

		recoveryDependencies, err :=
			newFlexAuthorizationRecoveryDependencies(
				cfg.Stripe.SecretKey,
			)
		if err != nil {
			log.Error(
				"failed to initialize FLEX authorization recovery dependencies",
				"error",
				err,
			)
			return 1
		}

		recoveryLifecycle, err :=
			buildFlexAuthorizationRecoveryLifecycle(
				db,
				recoveryDependencies,
				5*time.Minute,
				25,
			)
		if err != nil {
			log.Error(
				"failed to build FLEX authorization recovery lifecycle",
				"error",
				err,
			)
			return 1
		}

		recoveryController, err =
			newFlexAuthorizationRecoveryController(
				cfg,
				recoveryLifecycle,
			)
		if err != nil {
			log.Error(
				"failed to configure FLEX authorization recovery controller",
				"error",
				err,
			)
			return 1
		}

		// This deferred barrier also protects early startup failures.
		// It runs before the deferred PostgreSQL pool closure.
		defer func() {
			if err := waitForFlexShutdown(
				recoveryController,
				30*time.Second,
			); err != nil {
				log.Error(
					"FLEX authorization recovery cleanup encountered an error",
					"error",
					err,
				)
				exitCode = 1
			}
		}()

		if err := recoveryController.Start(
			context.Background(),
			flexauthorization.AuthorizationRecoverySchedulerOptions{
				Interval: 30 * time.Second,
				OnError: func(err error) {
					log.Error(
						"FLEX authorization recovery cycle error",
						"error",
						err,
					)
				},
			},
			func(err error) {
				log.Error(
					"FLEX authorization recovery worker exited unexpectedly",
					"error",
					err,
				)

				select {
				case recoveryWorkerFailure <- err:
				default:
				}
			},
		); err != nil {
			log.Error(
				"failed to start FLEX authorization recovery worker",
				"error",
				err,
			)
			return 1
		}

		log.Info("FLEX authorization recovery worker enabled")
	} else {
		log.Info("FLEX authorization recovery worker disabled")
	}

	// ----------------------------------------------------------------------
	// Automatic Redispatch Worker
	// ----------------------------------------------------------------------

	// Worker context is independent from individual HTTP requests and remains
	// active for the lifetime of the CONNECT backend process.
	redispatchCtx, cancelRedispatch := context.WithCancel(
		context.Background(),
	)

	redispatchDone := make(chan struct{})

	// Ensure startup errors cannot close PostgreSQL while
	// redispatch still has an active database operation.
	defer func() {
		cancelRedispatch()
		<-redispatchDone
	}()

	go func() {
		defer close(redispatchDone)

		dispatchService.StartRedispatchWorker(
			redispatchCtx,
			dispatchservice.RedispatchWorkerOptions{
				Interval:  2 * time.Second,
				BatchSize: 100,
			},
		)
	}()

	// Tracks driver shifts, live maps, and online/offline status values.
	presenceService := presence.NewService(
		presence.Dependencies{
			DB:          db,
			Config:      cfg,
			Users:       userRepo,
			Drivers:     driverRepo,
			Presence:    driverPresenceRepo,
			Assignments: driverAssignmentRepo,
			Compliance:  driverComplianceService,
			ComplianceForDB: func(db postgresrepo.DBTX) presence.ComplianceEvaluator {
				return drivercomplianceservice.NewService(
					drivercomplianceservice.Dependencies{
						Drivers:   postgresrepo.NewDriverRepositoryWithDB(db),
						Documents: postgresrepo.NewDriverDocumentRepositoryWithDB(db),
					},
				)
			},
		},
	)

	// Reconciles persisted idle presence with heartbeat freshness. Dispatch
	// already rejects stale heartbeats independently; this worker keeps the
	// persisted operational state truthful as well.
	stalePresenceCtx, cancelStalePresence := context.WithCancel(
		context.Background(),
	)

	stalePresenceDone := make(chan struct{})

	// Ensure startup errors cannot close PostgreSQL while
	// stale-presence reconciliation is still running.
	defer func() {
		cancelStalePresence()
		<-stalePresenceDone
	}()

	go func() {
		defer close(stalePresenceDone)

		presenceService.StartStalePresenceWorker(
			stalePresenceCtx,
			presence.StalePresenceWorkerOptions{
				Interval: cfg.Presence.HeartbeatTimeout / 2,
				OnError: func(err error) {
					log.Error(
						"stale presence reconciliation failed",
						"error",
						err,
					)
				},
			},
		)
	}()

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
	authHandler := api.NewAuthHandler(authService, cfg)
	userHandler := api.NewUserHandler(userRoleRepo)
	driverPresenceHandler := api.NewDriverPresenceHandler(presenceService)
	driverEarningsHandler :=
		api.NewDriverEarningsHandler(
			driverEarningService,
		)
	companyHandler := api.NewCompanyHandler(companyService)
	branchHandler := api.NewBranchHandler(branchService)
	driverAssignmentHandler := api.NewDriverAssignmentHandler(assignmentService)
	fleetHandler := api.NewFleetHandler(fleetService)
	vehicleHandler := api.NewVehicleHandler(vehicleService)
	driverHandler := api.NewDriverHandler(driverService)
	driverDocumentHandler := api.NewDriverDocumentHandler(
		driverDocumentService,
	)
	realtimeBroker := realtime.NewBroker()
	realtimeShutdown := make(chan struct{})

	tripStreamHandler := api.NewTripStreamHandler(
		tripService,
		realtimeBroker,
		realtimeShutdown,
	)
	tripHandler := api.NewTripHandlerWithRealtime(
		tripService,
		realtimeBroker,
	)

	paymentHandler := api.NewPaymentHandler(paymentService)

	rideRequestHandler := api.NewRideRequestHandler(
		rideRequestService,
	)

	serviceCategoryHandler := api.NewServiceCategoryHandler(
		serviceCategoryRepo,
	)

	fareEstimateHandler := api.NewFareEstimateHandler(
		fareEstimateService,
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
		cfg,
		authHandler,
		authMiddleware,
		rbacMiddleware,
		userHandler,
		driverPresenceHandler,
		driverEarningsHandler,
		driverAssignmentHandler,
		branchHandler,
		companyHandler,
		fleetHandler,
		vehicleHandler,
		driverHandler,
		driverDocumentHandler,
		tripHandler,
		tripStreamHandler,
		paymentHandler,
		paymentTransactionHandler,
		paymentExecutionHandler,
		paymentCallbackHandler,
		flexWebhookHandler,
		rideRequestHandler,
		serviceCategoryHandler,
		fareEstimateHandler,
		dispatchHandler,
		passengerAuthorizationHandler,
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

			select {
			case serverFailure <- err:
			default:
			}
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
	defer signal.Stop(quit)

	// Pauses execution stream here until a cancellation signal arrives.
	shutdown := waitForSupervisedShutdownDecision(
		quit,
		flexWorkerFailure,
		recoveryWorkerFailure,
		serverFailure,
	)

	shutdownReason := shutdown.Reason
	emergencyShutdown := shutdown.Emergency

	if shutdown.Err != nil {
		log.Error(
			"CONNECT shutting down after internal failure",
			"reason",
			shutdownReason,
			"error",
			shutdown.Err,
		)
	}

	log.Info(
		"CONNECT shutdown initiated",
		"reason",
		shutdownReason,
	)

	log.Info("Shutdown signal received")

	// Signal long-lived realtime streams to terminate before
	// waiting for the HTTP server to shut down.
	close(realtimeShutdown)

	// A nil *flexSettlementController must not become a
	// non-nil flexShutdownWaiter interface when FLEX is disabled.
	var shutdownFlexWorker flexShutdownWaiter
	if flexController != nil {
		shutdownFlexWorker = flexController
	}

	var shutdownRecoveryWorker flexShutdownWaiter
	if recoveryController != nil {
		shutdownRecoveryWorker = recoveryController
	}

	// Complete all shutdown barriers before deferred database cleanup.
	shutdownErr := coordinateShutdown(shutdownDependencies{
		HTTPServer:          server,
		Emergency:           emergencyShutdown,
		HTTPTimeout:         10 * time.Second,
		FlexWorker:          shutdownFlexWorker,
		FlexTimeout:         30 * time.Second,
		RecoveryWorker:      shutdownRecoveryWorker,
		RecoveryTimeout:     30 * time.Second,
		CancelRedispatch:    cancelRedispatch,
		RedispatchDone:      redispatchDone,
		CancelStalePresence: cancelStalePresence,
		StalePresenceDone:   stalePresenceDone,
	})

	if shutdownErr != nil {
		log.Error(
			"CONNECT shutdown completed with errors",
			"error",
			shutdownErr,
		)
	}

	log.Info("CONNECT Backend stopped")

	return shutdownExitCode(shutdown, shutdownErr)
}
