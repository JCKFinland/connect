package auth

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
)

// Dependencies contains all dependencies required by AuthService.
type Dependencies struct {
	Config *config.Config

	Users repository.UserRepository

	Roles repository.RoleRepository

	UserRoles repository.UserRoleRepository

	RefreshTokens repository.RefreshTokenRepository

	JWT *security.JWTService
}

// AuthService contains authentication business logic.
type AuthService struct {
	cfg *config.Config

	users repository.UserRepository

	roles repository.RoleRepository

	userRoles repository.UserRoleRepository

	refreshTokens repository.RefreshTokenRepository

	jwt *security.JWTService
}

// NewService creates a new AuthService.
func NewService(deps Dependencies) *AuthService {

	return &AuthService{
		cfg: deps.Config,

		users: deps.Users,

		roles: deps.Roles,

		userRoles: deps.UserRoles,

		refreshTokens: deps.RefreshTokens,

		jwt: deps.JWT,
	}
}
