package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	driverearning "github.com/JCKFinland/connect/backend/internal/services/driver_earning"
)

type driverEarningsAPIRepository struct {
	repository.DriverEarningRepository

	requestedSummaryDriverID string
	requestedRecentDriverID  string
}

func (r *driverEarningsAPIRepository) GetSummaryByDriverID(
	_ context.Context,
	driverID string,
) (*models.DriverEarningsSummary, error) {
	r.requestedSummaryDriverID = driverID

	return &models.DriverEarningsSummary{
		GrossAmount: "29.70",
		NetAmount:   "29.70",
		Currency:    "EUR",
		TripCount:   1,
	}, nil
}

func (r *driverEarningsAPIRepository) ListRecentByDriverID(
	_ context.Context,
	driverID string,
	_ int,
) ([]models.DriverEarning, error) {
	r.requestedRecentDriverID = driverID

	return []models.DriverEarning{
		{
			TripID:      "trip-123",
			DriverID:    driverID,
			GrossAmount: "29.70",
			NetAmount:   "29.70",
			Currency:    "EUR",
		},
	}, nil
}

func TestDriverEarningsDashboardUsesAuthenticatedUser(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	const userID = "authenticated-driver-user"

	repo := &driverEarningsAPIRepository{}

	service := driverearning.NewService(
		driverearning.Dependencies{
			Earnings: repo,
		},
	)

	handler := NewDriverEarningsHandler(service)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/driver/earnings",
		nil,
	)

	middleware.SetCurrentUser(
		c,
		&models.User{
			BaseModel: models.BaseModel{
				ID: userID,
			},
		},
	)

	handler.GetDashboard(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP status %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if repo.requestedSummaryDriverID != userID {
		t.Fatalf(
			"summary driver ID mismatch: got %s want %s",
			repo.requestedSummaryDriverID,
			userID,
		)
	}

	if repo.requestedRecentDriverID != userID {
		t.Fatalf(
			"recent driver ID mismatch: got %s want %s",
			repo.requestedRecentDriverID,
			userID,
		)
	}
}

func TestDriverEarningsDashboardRequiresAuthenticatedUser(
	t *testing.T,
) {
	gin.SetMode(gin.TestMode)

	repo := &driverEarningsAPIRepository{}

	service := driverearning.NewService(
		driverearning.Dependencies{
			Earnings: repo,
		},
	)

	handler := NewDriverEarningsHandler(service)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodGet,
		"/api/v1/driver/earnings",
		nil,
	)

	handler.GetDashboard(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP status %d, got %d: %s",
			http.StatusUnauthorized,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if repo.requestedSummaryDriverID != "" {
		t.Fatal(
			"earnings repository must not be queried without an authenticated user",
		)
	}

	if repo.requestedRecentDriverID != "" {
		t.Fatal(
			"recent earnings repository must not be queried without an authenticated user",
		)
	}
}
