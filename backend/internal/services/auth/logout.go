package auth

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/security"
)

// Logout revokes a refresh token.
func (s *AuthService) Logout(
	ctx context.Context,
	req RefreshTokenRequest,
) error {

	tokenHash := security.HashRefreshToken(req.RefreshToken)

	fmt.Println("Refresh Token :", req.RefreshToken)
	fmt.Println("Computed Hash :", tokenHash)

	return s.refreshTokens.DeleteByHash(ctx, tokenHash)
}
