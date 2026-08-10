package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List retrieves trips using filters and pagination.
func (s *tripService) List(
	ctx context.Context,
	companyID string,
	branchID string,
	status string,
	driverID string,
	customerID string,
	limit int,
	offset int,
) ([]*models.Trip, error) {
	if limit < 0 {
		return nil, fmt.Errorf("limit cannot be negative")
	}

	if offset < 0 {
		return nil, fmt.Errorf("offset cannot be negative")
	}

	// Apply service-level defaults.
	if limit == 0 {
		limit = 50
	}

	// Prevent excessive requests.
	if limit > 100 {
		limit = 100
	}

	trips, err := s.repo.List(
		ctx,
		companyID,
		branchID,
		status,
		driverID,
		customerID,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}

	return trips, nil
}