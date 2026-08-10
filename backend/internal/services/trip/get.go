package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// GetByID retrieves a trip by ID.
func (s *tripService) GetByID(
	ctx context.Context,
	id string,
) (*models.Trip, error) {
	if id == "" {
		return nil, fmt.Errorf("trip ID is required")
	}

	trip, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return trip, nil
}