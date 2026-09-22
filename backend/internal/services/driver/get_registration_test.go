package driver

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

func TestGetRegistrationReturnsAuthenticatedUsersDriver(t *testing.T) {
	expected := &models.Driver{
		BaseModel: models.BaseModel{
			ID: "driver-1",
		},
		UserID:     "user-1",
		Status:     "PENDING_VERIFICATION",
		IsVerified: false,
		IsActive:   true,
	}

	driverRepo := &driverRepositoryStub{
		existing: expected,
	}

	service := NewService(
		Dependencies{
			Drivers: driverRepo,
		},
	)

	user := &models.User{
		BaseModel: models.BaseModel{
			ID: "user-1",
		},
	}

	got, err := service.GetRegistration(
		context.Background(),
		user,
	)
	if err != nil {
		t.Fatalf("get registration: %v", err)
	}

	if got != expected {
		t.Fatal("expected authenticated user's driver registration")
	}

	if got.UserID != user.ID {
		t.Fatalf(
			"expected user ID %q, got %q",
			user.ID,
			got.UserID,
		)
	}
}

func TestGetRegistrationReturnsDriverNotFound(t *testing.T) {
	driverRepo := &driverRepositoryStub{
		getErr: repository.ErrNotFound,
	}

	service := NewService(
		Dependencies{
			Drivers: driverRepo,
		},
	)

	user := &models.User{
		BaseModel: models.BaseModel{
			ID: "user-1",
		},
	}

	_, err := service.GetRegistration(
		context.Background(),
		user,
	)

	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf(
			"expected ErrDriverNotFound, got %v",
			err,
		)
	}
}

func TestGetRegistrationRejectsMissingAuthenticatedUser(t *testing.T) {
	service := NewService(
		Dependencies{
			Drivers: &driverRepositoryStub{},
		},
	)

	_, err := service.GetRegistration(
		context.Background(),
		nil,
	)

	if !errors.Is(err, ErrInvalidDriver) {
		t.Fatalf(
			"expected ErrInvalidDriver, got %v",
			err,
		)
	}
}

func TestGetRegistrationPropagatesRepositoryError(t *testing.T) {
	repositoryErr := errors.New("database unavailable")

	driverRepo := &driverRepositoryStub{
		getErr: repositoryErr,
	}

	service := NewService(
		Dependencies{
			Drivers: driverRepo,
		},
	)

	user := &models.User{
		BaseModel: models.BaseModel{
			ID: "user-1",
		},
	}

	_, err := service.GetRegistration(
		context.Background(),
		user,
	)

	if !errors.Is(err, repositoryErr) {
		t.Fatalf(
			"expected repository error, got %v",
			err,
		)
	}
}
