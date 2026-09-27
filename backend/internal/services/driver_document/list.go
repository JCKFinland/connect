package driver_document

import (
	"context"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// List returns all active regulatory documents belonging to the supplied
// operational driver.
func (s *Service) List(
	ctx context.Context,
	driverID string,
) ([]models.DriverDocument, error) {
	driverID = strings.TrimSpace(driverID)

	if driverID == "" {
		return nil, ErrInvalidDocument
	}

	if s.documents == nil {
		return nil, fmt.Errorf(
			"driver document repository is not configured",
		)
	}

	documents, err := s.documents.ListByDriver(
		ctx,
		driverID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"list driver documents: %w",
			err,
		)
	}

	if documents == nil {
		return []models.DriverDocument{}, nil
	}

	return documents, nil
}
