package rbac

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

type Service struct {
	permissions repository.PermissionRepository
}

func NewService(
	permissions repository.PermissionRepository,
) *Service {

	return &Service{
		permissions: permissions,
	}
}

// HasPermission returns true if the user possesses the specified permission.
func (s *Service) HasPermission(
	ctx context.Context,
	userID string,
	permission string,
) (bool, error) {

	return s.permissions.HasPermission(
		ctx,
		userID,
		permission,
	)
}
