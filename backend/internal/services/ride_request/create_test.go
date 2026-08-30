package ride_request

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type createRideRequestTestRepository struct {
	created *models.RideRequest
}

type createServiceCategoryTestRepository struct {
	category *models.ServiceCategory
	err      error
}

func (r *createServiceCategoryTestRepository) Create(
	ctx context.Context,
	category *models.ServiceCategory,
) error {
	return nil
}

func (r *createServiceCategoryTestRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.ServiceCategory, error) {
	if r.err != nil {
		return nil, r.err
	}

	if r.category == nil || r.category.ID != id {
		return nil, repository.ErrNotFound
	}

	return r.category, nil
}

func (r *createServiceCategoryTestRepository) GetByCode(
	ctx context.Context,
	code string,
) (*models.ServiceCategory, error) {
	return nil, repository.ErrNotFound
}

func (r *createServiceCategoryTestRepository) List(
	ctx context.Context,
	activeOnly bool,
) ([]models.ServiceCategory, error) {
	return nil, nil
}

func (r *createRideRequestTestRepository) Create(
	ctx context.Context,
	request *models.RideRequest,
) error {

	r.created = request

	return nil
}

func (r *createRideRequestTestRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.RideRequest, error) {

	return nil, repository.ErrNotFound
}

func (r *createRideRequestTestRepository) List(
	ctx context.Context,
	customerID string,
	status string,
	limit int,
	offset int,
) ([]*models.RideRequest, error) {

	return nil, nil
}

func (r *createRideRequestTestRepository) Update(
	ctx context.Context,
	request *models.RideRequest,
) error {

	return nil
}

func (r *createRideRequestTestRepository) Delete(
	ctx context.Context,
	id string,
) error {

	return nil
}

func (r *createRideRequestTestRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
) error {

	return nil
}

func (r *createRideRequestTestRepository) GetByIDForUpdate(
	ctx context.Context,
	id string,
) (*models.RideRequest, error) {

	return nil, repository.ErrNotFound
}

func (r *createRideRequestTestRepository) ScheduleDispatchRetry(
	ctx context.Context,
	rideRequestID string,
	attemptedAt time.Time,
) (int, time.Time, error) {

	return 0, time.Time{}, nil
}

func (r *createRideRequestTestRepository) ResetDispatchRetry(
	ctx context.Context,
	rideRequestID string,
) error {

	return nil
}

func (r *createRideRequestTestRepository) ExpireDispatchableRideRequests(
	ctx context.Context,
	now time.Time,
) ([]string, error) {

	return nil, nil
}

func TestCreateAppliesDefaultMatchingLifetime(t *testing.T) {
	repo := &createRideRequestTestRepository{}

	const defaultLifetime = 10 * time.Minute

	service := NewService(
		Dependencies{
			Config: &config.Config{
				RideRequest: config.RideRequestConfig{
					DefaultMatchingLifetime: 10 * time.Minute,
				},
			},
			RideRequests: repo,
			ServiceCategories: &createServiceCategoryTestRepository{
				category: validServiceCategory(),
			},
		},
	)

	beforeCreate := time.Now().UTC()

	request, err := service.Create(
		context.Background(),
		validCreateRideRequest(),
	)
	if err != nil {
		t.Fatalf(
			"create ride request: %v",
			err,
		)
	}

	afterCreate := time.Now().UTC()

	if request == nil {
		t.Fatal(
			"expected created ride request",
		)
	}

	if request.ExpiresAt == nil {
		t.Fatal(
			"expected default expires_at to be assigned",
		)
	}

	earliestExpected :=
		beforeCreate.Add(defaultLifetime)

	latestExpected :=
		afterCreate.Add(defaultLifetime)

	if request.ExpiresAt.Before(
		earliestExpected,
	) {
		t.Fatalf(
			"expires_at %v is earlier than expected %v",
			*request.ExpiresAt,
			earliestExpected,
		)
	}

	if request.ExpiresAt.After(
		latestExpected,
	) {
		t.Fatalf(
			"expires_at %v is later than expected %v",
			*request.ExpiresAt,
			latestExpected,
		)
	}

	if repo.created == nil {
		t.Fatal(
			"expected ride request to be persisted",
		)
	}

	if repo.created.ExpiresAt == nil {
		t.Fatal(
			"expected persisted ride request expiry",
		)
	}

	if repo.created.Status != StatusPending {
		t.Fatalf(
			"expected status %s, got %s",
			StatusPending,
			repo.created.Status,
		)
	}
}

func TestCreatePreservesFutureExplicitExpiry(t *testing.T) {
	repo := &createRideRequestTestRepository{}

	service := NewService(
		Dependencies{
			Config: &config.Config{
				RideRequest: config.RideRequestConfig{
					DefaultMatchingLifetime: 10 * time.Minute,
				},
			},
			RideRequests: repo,
			ServiceCategories: &createServiceCategoryTestRepository{
				category: validServiceCategory(),
			},
		},
	)

	explicitExpiry :=
		time.Now().UTC().Add(
			25 * time.Minute,
		)

	req := validCreateRideRequest()
	req.ExpiresAt = &explicitExpiry

	request, err := service.Create(
		context.Background(),
		req,
	)
	if err != nil {
		t.Fatalf(
			"create ride request: %v",
			err,
		)
	}

	if request == nil {
		t.Fatal(
			"expected created ride request",
		)
	}

	if request.ExpiresAt == nil {
		t.Fatal(
			"expected explicit expires_at",
		)
	}

	if !request.ExpiresAt.Equal(
		explicitExpiry,
	) {
		t.Fatalf(
			"expected expiry %v, got %v",
			explicitExpiry,
			*request.ExpiresAt,
		)
	}

	if repo.created == nil {
		t.Fatal(
			"expected ride request to be persisted",
		)
	}

	if repo.created.ExpiresAt == nil ||
		!repo.created.ExpiresAt.Equal(
			explicitExpiry,
		) {

		t.Fatalf(
			"expected persisted expiry %v, got %v",
			explicitExpiry,
			repo.created.ExpiresAt,
		)
	}
}

func TestCreateRejectsExpiredExplicitExpiry(t *testing.T) {
	repo := &createRideRequestTestRepository{}

	service := NewService(
		Dependencies{
			Config: &config.Config{
				RideRequest: config.RideRequestConfig{
					DefaultMatchingLifetime: 10 * time.Minute,
				},
			},
			RideRequests: repo,
			ServiceCategories: &createServiceCategoryTestRepository{
				category: validServiceCategory(),
			},
		},
	)

	expiredAt :=
		time.Now().UTC().Add(
			-1 * time.Minute,
		)

	req := validCreateRideRequest()
	req.ExpiresAt = &expiredAt

	request, err := service.Create(
		context.Background(),
		req,
	)

	if !errors.Is(
		err,
		ErrInvalidRideRequestExpiry,
	) {
		t.Fatalf(
			"expected ErrInvalidRideRequestExpiry, got request=%v err=%v",
			request,
			err,
		)
	}

	if request != nil {
		t.Fatalf(
			"expected no created request, got %+v",
			request,
		)
	}

	if repo.created != nil {
		t.Fatalf(
			"expected invalid ride not to be persisted, got %+v",
			repo.created,
		)
	}
}

func validCreateRideRequest() CreateRideRequestRequest {
	return CreateRideRequestRequest{
		CustomerID: "49c61249-8b7d-4afd-a559-6d54567ee164",

		PickupAddress: "Espoo Test Pickup",

		PickupLatitude: 60.2055,

		PickupLongitude: 24.6559,

		DestinationAddress: "Helsinki Central Station",

		DestinationLatitude: 60.1719,

		DestinationLongitude: 24.9414,

		RequestedVehicleType: "STANDARD",
		ServiceCategoryID:    validServiceCategory().ID,

		PassengerCount: 1,

		Notes: "Ride expiry policy test",
	}
}

func validServiceCategory() *models.ServiceCategory {
	return &models.ServiceCategory{
		BaseModel: models.BaseModel{
			ID: "11111111-1111-4111-8111-111111111111",
		},
		Code:     "BASIC",
		Name:     "Basic",
		IsActive: true,
	}
}

func TestCreateRejectsUnknownServiceCategory(t *testing.T) {
	repo := &createRideRequestTestRepository{}

	service := NewService(
		Dependencies{
			Config: &config.Config{
				RideRequest: config.RideRequestConfig{
					DefaultMatchingLifetime: 10 * time.Minute,
				},
			},
			RideRequests:      repo,
			ServiceCategories: &createServiceCategoryTestRepository{},
		},
	)

	request, err := service.Create(
		context.Background(),
		validCreateRideRequest(),
	)

	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf(
			"expected repository.ErrNotFound, got request=%v err=%v",
			request,
			err,
		)
	}

	if request != nil {
		t.Fatalf(
			"expected no created request, got %+v",
			request,
		)
	}

	if repo.created != nil {
		t.Fatalf(
			"expected unknown category ride not to be persisted, got %+v",
			repo.created,
		)
	}
}

func TestCreateRejectsInactiveServiceCategory(t *testing.T) {
	repo := &createRideRequestTestRepository{}

	category := validServiceCategory()
	category.IsActive = false

	service := NewService(
		Dependencies{
			Config: &config.Config{
				RideRequest: config.RideRequestConfig{
					DefaultMatchingLifetime: 10 * time.Minute,
				},
			},
			RideRequests: repo,
			ServiceCategories: &createServiceCategoryTestRepository{
				category: category,
			},
		},
	)

	request, err := service.Create(
		context.Background(),
		validCreateRideRequest(),
	)

	if err == nil {
		t.Fatal("expected inactive service category error")
	}

	if request != nil {
		t.Fatalf(
			"expected no created request, got %+v",
			request,
		)
	}

	if repo.created != nil {
		t.Fatalf(
			"expected inactive category ride not to be persisted, got %+v",
			repo.created,
		)
	}
}
