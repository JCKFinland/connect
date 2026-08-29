package fare

import (
	"github.com/JCKFinland/connect/backend/internal/models"
)

// Service defines deterministic fare calculation operations.
type Service interface {
	Calculate(
		input CalculationInput,
	) (*models.TripFare, error)
}

type fareService struct{}

// NewService creates a new fare calculation service.
func NewService() Service {
	return &fareService{}
}

var _ Service = (*fareService)(nil)
