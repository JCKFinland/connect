package auth

import (
	"context"
	"errors"
	"time"

	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
)

// Refresh rotates a refresh token and returns a new token pair.
func (s *AuthService) Refresh(
	ctx context.Context,
	req RefreshTokenRequest,
) (*AuthResponse, error) {

	// Hash the incoming refresh token.
	tokenHash := security.HashRefreshToken(req.RefreshToken)

	// Look up the stored refresh token.
	storedToken, err := s.refreshTokens.GetByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrRefreshTokenNotFound) {
			return nil, repository.ErrInvalidRefreshToken
		}
		return nil, err
	}

	// Check whether the refresh token has expired.
	if time.Now().UTC().After(storedToken.ExpiresAt) {

		// Best-effort cleanup.
		_ = s.refreshTokens.DeleteByHash(ctx, tokenHash)

		return nil, repository.ErrInvalidRefreshToken
	}

	// Load the user.
	user, err := s.users.GetByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, err
	}

	// Ensure the account is active.
	if !user.IsActive {
		return nil, repository.ErrAccountDisabled
	}

	// Generate a new access/refresh token pair.
	tokenPair, err := security.GenerateTokenPair(s.jwt, user)
	if err != nil {
		return nil, err
	}

	// Store the new refresh token.
	newHash := security.HashRefreshToken(tokenPair.RefreshToken)

	newRefresh := *storedToken
	newRefresh.ID = ""
	newRefresh.TokenHash = newHash
	newRefresh.ExpiresAt = time.Now().UTC().Add(
		s.cfg.JWT.RefreshTokenDuration,
	)

	if err := s.refreshTokens.Create(ctx, &newRefresh); err != nil {
		return nil, err
	}

	// Delete the old refresh token.
	if err := s.refreshTokens.DeleteByHash(ctx, tokenHash); err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.cfg.JWT.AccessTokenDuration.Seconds()),
	}, nil
}
