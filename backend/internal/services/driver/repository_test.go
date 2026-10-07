package driver

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// driverRepositoryStub is shared by driver service unit tests that exercise
// repository-backed read behavior without a database.
type driverRepositoryStub struct {
	existing  *models.Driver
	created   *models.Driver
	getErr    error
	createErr error
}

func (r *driverRepositoryStub) Create(
	ctx context.Context,
	driver *models.Driver,
) error {
	if r.createErr != nil {
		return r.createErr
	}

	r.created = driver
	return nil
}

func (r *driverRepositoryStub) GetByID(
	ctx context.Context,
	id string,
) (*models.Driver, error) {
	return nil, repository.ErrNotFound
}

func (r *driverRepositoryStub) GetByIDForCompanyMember(
	ctx context.Context,
	_ string,
	id string,
) (*models.Driver, error) {
	return r.GetByID(ctx, id)
}

func (r *driverRepositoryStub) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.Driver, error) {
	return r.GetByID(ctx, id)
}

func (r *driverRepositoryStub) GetByUserID(
	ctx context.Context,
	userID string,
) (*models.Driver, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}

	if r.existing == nil {
		return nil, repository.ErrNotFound
	}

	return r.existing, nil
}

func (r *driverRepositoryStub) List(
	ctx context.Context,
) ([]models.Driver, error) {
	return nil, nil
}

func (r *driverRepositoryStub) ListForCompanyMember(
	ctx context.Context,
	_ string,
) ([]models.Driver, error) {
	return r.List(ctx)
}

func (r *driverRepositoryStub) UpdateDetails(
	ctx context.Context,
	driver *models.Driver,
) error {
	return nil
}

func (r *driverRepositoryStub) Update(
	ctx context.Context,
	driver *models.Driver,
) error {
	return nil
}

func (r *driverRepositoryStub) Delete(
	ctx context.Context,
	id string,
) error {
	return nil
}
