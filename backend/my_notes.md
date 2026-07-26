# Backend Folder Structure
backend/
│
├── cmd/
│   └── api/
│       └── main.go
│
├── internal/
│   ├── api/
│   ├── auth/
│   ├── booking/
│   ├── compliance/
│   ├── config/
│   ├── database/
│   ├── dispatch/
│   ├── driver/
│   ├── fleet/
│   ├── middleware/
│   ├── models/
│   ├── notification/
│   ├── passenger/
│   ├── payment/
│   ├── pricing/
│   ├── reporting/
│   ├── trip/
│   ├── user/
│   └── vehicle/
│
├── pkg/
│   ├── logger/
│   ├── response/
│   └── validator/
│
├── migrations/
│
├── scripts/
│
├── configs/
│
├── docs/
│
├── tests/
│
├── go.mod
└── go.sum

# Why this structure?

It follows a layered, modular approach:

cmd/ → application entry points
internal/ → business domains (protected from external imports)
pkg/ → reusable utilities
migrations/ → database schema changes
configs/ → configuration files
tests/ → integration and end-to-end tests

This organization scales well as the project grows.

# Development Standards

From this point forward, we'll use these conventions:

Every API under /api/v1
JSON request/response format
UUIDs for identifiers
UTC timestamps
Structured logging
Dependency injection where appropriate
Configuration through environment variables
No hard-coded secrets


# Configuration Management
backend/internal/config/
├── config.go
├── env.go
└── validation.go

When complete:

.env loads successfully
Missing required values cause startup to fail with clear errors
Configuration is available throughout the application

# Responsibilities:

Load .env
Validate required environment variables
Store application configuration
Make configuration available throughout the application

# Environment Files
backend/
├── .env
├── .env.example
└── .gitignore


# Database Package

backend/internal/database/
├── postgres.go
├── migrations.go
└── health.go

# Responsibilities:

Open PostgreSQL connection
Connection pooling
Health checks
Migration support

# Logger Package
backend/pkg/logger/
├── logger.go
└── middleware.go

We'll configure:

log/slog
Development logging
Production logging
Request IDs
Startup logs
Error logs

We'll use Go's built-in log/slog.

# The logger should:

Output structured JSON in production.
Output readable text during development.
Include timestamps.
Include log levels.
Include request IDs where available.

# API Package
backend/internal/api/
├── router.go
├── routes.go
└── health.go
This keeps routing separate from handlers.

# Standard API Response Format
Every endpoint should return a consistent structure.

Successful response:

{
  "success": true,
  "message": "OK",
  "data": {},
  "meta": {}
}

# Error response:

{
  "success": false,
  "message": "Validation failed",
  "errors": [
    {
      "field": "email",
      "message": "Email is required"
    }
  ]
}

This consistency makes client development much easier.


# NOTE

config.go → Loads application configuration
env.go → Reads environment variables
validation.go → Validates configuration at startup
postgres.go → Creates the PostgreSQL connection pool
router.go → Builds the Gin router
routes.go → Registers API routes
health.go → Returns service health
logger.go → Configures structured logging
middleware.go → Logs every HTTP request


# PostgreSQL
internal/database/

We'll implement:

Database connection
Connection pool
Health check
Automatic reconnection handling
Graceful shutdown

# API
internal/api/

We'll implement:

Gin router
API versioning
Middleware
Health endpoint
JSON response helpers

# Application Startup
cmd/api/main.go

When we reach this step, running: go run cmd/api/main.go

should produce something like:

=========================================
CONNECT Backend
Version: v0.1.0-alpha
Environment: development
=========================================

Loading configuration...
Configuration loaded.

Initializing logger...
Logger initialized.

Connecting to PostgreSQL...
Database connected.

Registering routes...
Routes registered.

Server listening on http://localhost:8080

READY

That will be our first fully functioning backend.


# Engineering standard that will benefit us throughout the project.
Every Package Should Have One Clear Responsibility


