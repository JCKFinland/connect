package auth

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/security"
)

// Logout revokes a refresh token.
func (s *AuthService) Logout(
	ctx context.Context,
	req RefreshTokenRequest,
) error {
	tokenHash := security.HashRefreshToken(
		req.RefreshToken,
	)

	return s.refreshTokens.DeleteByHash(
		ctx,
		tokenHash,
	)
}
