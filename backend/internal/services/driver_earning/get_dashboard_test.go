package driver_earning

import (
	"context"
	"errors"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type dashboardEarningRepository struct {
	repository.DriverEarningRepository

	summary *models.DriverEarningsSummary
	recent  []models.DriverEarning

	summaryErr error
	recentErr  error

	recentCalled bool

	summaryDriverID string
	recentDriverID  string
	recentLimit     int
}

func (r *dashboardEarningRepository) GetSummaryByDriverID(
	_ context.Context,
	driverID string,
) (*models.DriverEarningsSummary, error) {
	r.summaryDriverID = driverID

	if r.summaryErr != nil {
		return nil, r.summaryErr
	}

	return r.summary, nil
}

func (r *dashboardEarningRepository) ListRecentByDriverID(
	_ context.Context,
	driverID string,
	limit int,
) ([]models.DriverEarning, error) {
	r.recentCalled = true
	r.recentDriverID = driverID
	r.recentLimit = limit

	if r.recentErr != nil {
		return nil, r.recentErr
	}

	return r.recent, nil
}

func TestGetDashboardUsesAuthenticatedUserID(
	t *testing.T,
) {
	const userID = "driver-user-123"

	repo := &dashboardEarningRepository{
		summary: &models.DriverEarningsSummary{
			GrossAmount: "29.70",
			NetAmount:   "29.70",
			Currency:    "EUR",
			TripCount:   1,
		},
		recent: []models.DriverEarning{
			{
				TripID:      "trip-123",
				DriverID:    userID,
				GrossAmount: "29.70",
				NetAmount:   "29.70",
				Currency:    "EUR",
			},
		},
	}

	service := NewService(
		Dependencies{
			Earnings: repo,
		},
	)

	dashboard, err := service.GetDashboard(
		context.Background(),
		userID,
	)
	if err != nil {
		t.Fatalf(
			"get driver earnings dashboard: %v",
			err,
		)
	}

	if repo.summaryDriverID != userID {
		t.Fatalf(
			"summary driver ID mismatch: got %s want %s",
			repo.summaryDriverID,
			userID,
		)
	}

	if repo.recentDriverID != userID {
		t.Fatalf(
			"recent driver ID mismatch: got %s want %s",
			repo.recentDriverID,
			userID,
		)
	}

	if repo.recentLimit != recentEarningsLimit {
		t.Fatalf(
			"recent earnings limit mismatch: got %d want %d",
			repo.recentLimit,
			recentEarningsLimit,
		)
	}

	if dashboard.Summary.NetAmount != "29.70" {
		t.Fatalf(
			"dashboard net amount mismatch: got %s want 29.70",
			dashboard.Summary.NetAmount,
		)
	}

	if len(dashboard.RecentEarnings) != 1 {
		t.Fatalf(
			"recent earnings count mismatch: got %d want 1",
			len(dashboard.RecentEarnings),
		)
	}

	if dashboard.RecentEarnings[0].DriverID != userID {
		t.Fatalf(
			"recent earning driver ID mismatch: got %s want %s",
			dashboard.RecentEarnings[0].DriverID,
			userID,
		)
	}
}

func TestGetDashboardStopsWhenSummaryFails(
	t *testing.T,
) {
	const userID = "driver-user-123"

	expectedErr := errors.New("database unavailable")

	repo := &dashboardEarningRepository{
		summaryErr: expectedErr,
	}

	service := NewService(
		Dependencies{
			Earnings: repo,
		},
	)

	dashboard, err := service.GetDashboard(
		context.Background(),
		userID,
	)

	if dashboard != nil {
		t.Fatal(
			"expected nil dashboard when summary retrieval fails",
		)
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected wrapped repository error, got %v",
			err,
		)
	}

	if repo.summaryDriverID != userID {
		t.Fatalf(
			"summary driver ID mismatch: got %s want %s",
			repo.summaryDriverID,
			userID,
		)
	}

	if repo.recentCalled {
		t.Fatal(
			"recent earnings must not be queried after summary failure",
		)
	}
}

func TestGetDashboardReturnsErrorWhenRecentEarningsFail(
	t *testing.T,
) {
	const userID = "driver-user-123"

	expectedErr := errors.New("recent earnings unavailable")

	repo := &dashboardEarningRepository{
		summary: &models.DriverEarningsSummary{
			GrossAmount: "29.70",
			NetAmount:   "29.70",
			Currency:    "EUR",
			TripCount:   1,
		},
		recentErr: expectedErr,
	}

	service := NewService(
		Dependencies{
			Earnings: repo,
		},
	)

	dashboard, err := service.GetDashboard(
		context.Background(),
		userID,
	)

	if dashboard != nil {
		t.Fatal(
			"expected nil dashboard when recent earnings retrieval fails",
		)
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected wrapped repository error, got %v",
			err,
		)
	}

	if repo.summaryDriverID != userID {
		t.Fatalf(
			"summary driver ID mismatch: got %s want %s",
			repo.summaryDriverID,
			userID,
		)
	}

	if !repo.recentCalled {
		t.Fatal(
			"expected recent earnings repository to be queried",
		)
	}

	if repo.recentDriverID != userID {
		t.Fatalf(
			"recent driver ID mismatch: got %s want %s",
			repo.recentDriverID,
			userID,
		)
	}

	if repo.recentLimit != recentEarningsLimit {
		t.Fatalf(
			"recent earnings limit mismatch: got %d want %d",
			repo.recentLimit,
			recentEarningsLimit,
		)
	}
}
