package auth

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
)

const customerRole = "CUSTOMER"

// Register creates a new customer account.
func (s *AuthService) Register(
	ctx context.Context,
	req RegisterRequest,
) (*UserResponse, error) {

	// Check whether email already exists.
	_, err := s.users.GetByEmail(ctx, req.Email)
	if err == nil {
		return nil, repository.ErrEmailAlreadyUsed
	}

	if !errors.Is(err, repository.ErrUserNotFound) {
		return nil, err
	}

	// Hash password.
	passwordHash, err := security.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	// Create user model.
	user := &models.User{
		Email:        req.Email,
		PasswordHash: passwordHash,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Phone:        req.Phone,
		IsActive:     true,
		IsVerified:   false,
	}

	// Save user.
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	// Load CUSTOMER role.
	role, err := s.roles.GetByName(ctx, customerRole)
	if err != nil {
		return nil, err
	}

	// Assign CUSTOMER role.
	if err := s.userRoles.AssignRole(ctx, user.ID, role.ID); err != nil {
		return nil, err
	}

	// Return safe response.
	return &UserResponse{
		ID:         user.ID,
		Email:      user.Email,
		FirstName:  user.FirstName,
		LastName:   user.LastName,
		Phone:      user.Phone,
		IsVerified: user.IsVerified,
	}, nil
}
