package repository

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type FarePricingProfileRepository interface {
	Create(
		ctx context.Context,
		profile *models.FarePricingProfile,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.FarePricingProfile, error)

	GetByVersion(
		ctx context.Context,
		companyID string,
		version string,
	) (*models.FarePricingProfile, error)

	ResolveEffective(
		ctx context.Context,
		companyID string,
		branchID *string,
		serviceCategoryID string,
		at time.Time,
	) (*models.FarePricingProfile, error)

	ListByCompanyID(
		ctx context.Context,
		companyID string,
		activeOnly bool,
	) ([]*models.FarePricingProfile, error)
}
