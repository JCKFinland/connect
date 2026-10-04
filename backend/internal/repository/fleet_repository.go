package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type FleetRepository interface {
	Create(
		ctx context.Context,
		fleet *models.Fleet,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.Fleet, error)

	// GetByIDForCompanyMember returns a fleet only when the user has
	// explicit membership in the fleet's company.
	GetByIDForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) (*models.Fleet, error)

	// List returns all non-deleted fleets across all companies.
	// This global surface is reserved for explicitly authorized callers.
	List(
		ctx context.Context,
	) ([]*models.Fleet, error)

	// ListForCompanyMember returns non-deleted fleets belonging only to
	// companies in which the user has explicit membership.
	ListForCompanyMember(
		ctx context.Context,
		userID string,
	) ([]*models.Fleet, error)

	ListActiveByCompanyAndBranch(
		ctx context.Context,
		companyID string,
		branchID string,
	) ([]*models.Fleet, error)

	Update(
		ctx context.Context,
		fleet *models.Fleet,
	) error

	Delete(
		ctx context.Context,
		id string,
	) error
}
