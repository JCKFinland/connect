package auth

import (
	"context"
	"errors"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
)

func (s *AuthService) Login(
	ctx context.Context,
	req LoginRequest,
) (*AuthResponse, error) {

	// Find user
	user, err := s.users.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, repository.ErrInvalidCredentials
		}
		return nil, err
	}

	// Verify password
	if err := security.VerifyPassword(
		user.PasswordHash,
		req.Password,
	); err != nil {
		return nil, repository.ErrInvalidCredentials
	}

	// Ensure account is active
	if !user.IsActive {
		return nil, repository.ErrAccountDisabled
	}

	// Generate tokens
	tokenPair, err := security.GenerateTokenPair(s.jwt, user)
	if err != nil {
		return nil, err
	}

	// Hash refresh token
	tokenHash := security.HashRefreshToken(tokenPair.RefreshToken)

	// Persist refresh token
	refreshToken := &models.RefreshToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.cfg.JWT.RefreshTokenDuration),
	}

	if err := s.refreshTokens.Create(ctx, refreshToken); err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.cfg.JWT.AccessTokenDuration.Seconds()),
	}, nil
}
