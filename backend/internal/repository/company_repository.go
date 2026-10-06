package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type CompanyRepository interface {
	Create(
		ctx context.Context,
		company *models.Company,
	) error

	UpdateDetails(
		ctx context.Context,
		company *models.Company,
	) error

	UpdateDetailsForCompanyMember(
		ctx context.Context,
		userID string,
		company *models.Company,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.Company, error)

	GetByIDForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) (*models.Company, error)

	List(
		ctx context.Context,
	) ([]*models.Company, error)

	ListForCompanyMember(
		ctx context.Context,
		userID string,
	) ([]*models.Company, error)

	ListActive(
		ctx context.Context,
	) ([]*models.Company, error)

	Archive(
		ctx context.Context,
		id string,
	) error

	ArchiveForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) error

	Deactivate(
		ctx context.Context,
		id string,
	) error

	DeactivateForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) error

	Reactivate(
		ctx context.Context,
		id string,
	) error

	ReactivateForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) error
}
