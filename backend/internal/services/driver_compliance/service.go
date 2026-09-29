package driver_compliance

import "github.com/JCKFinland/connect/backend/internal/repository"

// Dependencies contains the persistence dependencies required by the
// driver-compliance service.
type Dependencies struct {
	Drivers   repository.DriverRepository
	Documents repository.DriverDocumentRepository
}

// Service evaluates whether a driver satisfies CONNECT's operational
// regulatory requirements.
type Service struct {
	drivers   repository.DriverRepository
	documents repository.DriverDocumentRepository
}

// NewService creates a driver-compliance service.
func NewService(deps Dependencies) *Service {
	return &Service{
		drivers:   deps.Drivers,
		documents: deps.Documents,
	}
}
