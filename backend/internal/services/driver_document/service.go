package driver_document

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

// Dependencies contains the persistence dependencies required by the
// driver-document service.
type Dependencies struct {
	DB *pgxpool.Pool

	Documents repository.DriverDocumentRepository
	Drivers   repository.DriverRepository
}

// Service manages the lifecycle of driver regulatory-document metadata.
//
// Document binaries are stored outside PostgreSQL. This service manages
// metadata, document state, replacement, and later verification workflows.
type Service struct {
	db *pgxpool.Pool

	documents repository.DriverDocumentRepository
	drivers   repository.DriverRepository
}

// NewService creates a driver-document service.
func NewService(
	deps Dependencies,
) *Service {
	return &Service{
		db:        deps.DB,
		documents: deps.Documents,
		drivers:   deps.Drivers,
	}
}
