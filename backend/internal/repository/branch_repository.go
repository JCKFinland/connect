package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type BranchRepository interface {
	Create(
		ctx context.Context,
		branch *models.Branch,
	) error

	UpdateDetails(
		ctx context.Context,
		branch *models.Branch,
	) error

	UpdateDetailsForCompanyMember(
		ctx context.Context,
		userID string,
		branch *models.Branch,
	) error

	GetByID(
		ctx context.Context,
		id string,
	) (*models.Branch, error)

	GetByIDForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) (*models.Branch, error)

	List(
		ctx context.Context,
	) ([]*models.Branch, error)

	ListForCompanyMember(
		ctx context.Context,
		userID string,
	) ([]*models.Branch, error)

	ListActiveByCompanyID(
		ctx context.Context,
		companyID string,
	) ([]*models.Branch, error)

	// GetOwningCompanyID returns the current company ID for a non-deleted branch.
	// It is used to establish company -> branch lifecycle lock ordering before
	// performing the authoritative transactional re-read.
	GetOwningCompanyID(
		ctx context.Context,
		branchID string,
	) (string, error)

	HasNonDeletedByCompany(
		ctx context.Context,
		companyID string,
	) (bool, error)

	HasActiveByCompany(
		ctx context.Context,
		companyID string,
	) (bool, error)

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
