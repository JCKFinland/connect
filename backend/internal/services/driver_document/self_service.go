package driver_document

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// SubmitForUser submits or replaces a regulatory document belonging to the
// driver profile associated with the authenticated platform user.
func (s *Service) SubmitForUser(
	ctx context.Context,
	userID string,
	req SubmitDocumentRequest,
) (*models.DriverDocument, error) {
	driverID, err := s.driverIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.Submit(ctx, driverID, req)
}

// GetForUser returns one regulatory document belonging to the authenticated
// user's driver profile.
func (s *Service) GetForUser(
	ctx context.Context,
	userID string,
	documentID string,
) (*models.DriverDocument, error) {
	driverID, err := s.driverIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, driverID, documentID)
}

// ListForUser returns the regulatory documents belonging to the authenticated
// user's driver profile.
func (s *Service) ListForUser(
	ctx context.Context,
	userID string,
) ([]models.DriverDocument, error) {
	driverID, err := s.driverIDForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.List(ctx, driverID)
}

// driverIDForUser resolves an authenticated platform users.id to its
// authoritative drivers.id.
//
// Driver-document persistence must always use drivers.id because
// driver_documents.driver_id references drivers(id).
func (s *Service) driverIDForUser(
	ctx context.Context,
	userID string,
) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", ErrInvalidDocument
	}

	if s.drivers == nil {
		return "", fmt.Errorf(
			"driver repository is not configured",
		)
	}

	driver, err := s.drivers.GetByUserID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return "", ErrDriverNotFound
	}
	if err != nil {
		return "", fmt.Errorf(
			"get driver for authenticated user: %w",
			err,
		)
	}

	if driver == nil || strings.TrimSpace(driver.ID) == "" {
		return "", ErrDriverNotFound
	}

	return driver.ID, nil
}
