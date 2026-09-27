package driver_document

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type selfServiceDriverRepository struct {
	driver *models.Driver
	err    error

	getByUserIDCalled bool
	userID            string
}

func (r *selfServiceDriverRepository) Create(
	context.Context,
	*models.Driver,
) error {
	return nil
}

func (r *selfServiceDriverRepository) GetByID(
	context.Context,
	string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (r *selfServiceDriverRepository) GetByIDForUpdate(
	context.Context,
	string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (r *selfServiceDriverRepository) GetByUserID(
	_ context.Context,
	userID string,
) (*models.Driver, error) {
	r.getByUserIDCalled = true
	r.userID = userID

	if r.err != nil {
		return nil, r.err
	}

	return r.driver, nil
}

func (r *selfServiceDriverRepository) List(
	context.Context,
) ([]models.Driver, error) {
	return []models.Driver{}, nil
}

func (r *selfServiceDriverRepository) Update(
	context.Context,
	*models.Driver,
) error {
	return nil
}

func (r *selfServiceDriverRepository) Delete(
	context.Context,
	string,
) error {
	return nil
}

func TestDriverIDForUserResolvesAuthenticatedUserToDriverID(
	t *testing.T,
) {
	repo := &selfServiceDriverRepository{
		driver: &models.Driver{
			BaseModel: models.BaseModel{
				ID: "driver-123",
			},
			UserID: "user-123",
		},
	}

	service := NewService(Dependencies{
		Drivers: repo,
	})

	driverID, err := service.driverIDForUser(
		context.Background(),
		"  user-123  ",
	)
	if err != nil {
		t.Fatalf("resolve driver ID: %v", err)
	}

	if driverID != "driver-123" {
		t.Fatalf(
			"driver ID mismatch: got %q want %q",
			driverID,
			"driver-123",
		)
	}

	if !repo.getByUserIDCalled {
		t.Fatal("expected GetByUserID to be called")
	}

	if repo.userID != "user-123" {
		t.Fatalf(
			"user ID mismatch: got %q want %q",
			repo.userID,
			"user-123",
		)
	}
}

func TestDriverIDForUserReturnsDriverNotFound(
	t *testing.T,
) {
	repo := &selfServiceDriverRepository{
		err: repository.ErrNotFound,
	}

	service := NewService(Dependencies{
		Drivers: repo,
	})

	_, err := service.driverIDForUser(
		context.Background(),
		"user-123",
	)

	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf(
			"expected ErrDriverNotFound, got %v",
			err,
		)
	}
}

func TestDriverIDForUserRejectsEmptyUserID(
	t *testing.T,
) {
	repo := &selfServiceDriverRepository{}

	service := NewService(Dependencies{
		Drivers: repo,
	})

	_, err := service.driverIDForUser(
		context.Background(),
		"   ",
	)

	if !errors.Is(err, ErrInvalidDocument) {
		t.Fatalf(
			"expected ErrInvalidDocument, got %v",
			err,
		)
	}

	if repo.getByUserIDCalled {
		t.Fatal(
			"repository must not be queried for empty user ID",
		)
	}
}

func TestDriverIDForUserRejectsNilDriver(
	t *testing.T,
) {
	repo := &selfServiceDriverRepository{}

	service := NewService(Dependencies{
		Drivers: repo,
	})

	_, err := service.driverIDForUser(
		context.Background(),
		"user-123",
	)

	if !errors.Is(err, ErrDriverNotFound) {
		t.Fatalf(
			"expected ErrDriverNotFound, got %v",
			err,
		)
	}
}

func TestDriverIDForUserWrapsRepositoryError(
	t *testing.T,
) {
	repositoryErr := errors.New("database unavailable")

	repo := &selfServiceDriverRepository{
		err: repositoryErr,
	}

	service := NewService(Dependencies{
		Drivers: repo,
	})

	_, err := service.driverIDForUser(
		context.Background(),
		"user-123",
	)

	if !errors.Is(err, repositoryErr) {
		t.Fatalf(
			"expected wrapped repository error, got %v",
			err,
		)
	}
}
