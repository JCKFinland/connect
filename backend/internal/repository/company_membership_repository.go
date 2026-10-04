package repository

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
)

var (
	ErrCompanyMembershipAlreadyExists = errors.New("company membership already exists")
)

// CompanyMembershipRepository provides the canonical persistence boundary for
// explicit user authority within company tenants.
type CompanyMembershipRepository interface {
	Create(
		ctx context.Context,
		membership *models.CompanyMembership,
	) error

	Exists(
		ctx context.Context,
		userID string,
		companyID string,
	) (bool, error)

	ListByUserID(
		ctx context.Context,
		userID string,
	) ([]*models.CompanyMembership, error)
}
