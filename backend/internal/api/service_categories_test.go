package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/models"
)

type serviceCategoryTestRepository struct {
	categories    []models.ServiceCategory
	err           error
	gotActiveOnly bool
}

func (r *serviceCategoryTestRepository) Create(
	context.Context,
	*models.ServiceCategory,
) error {
	return nil
}

func (r *serviceCategoryTestRepository) GetByID(
	context.Context,
	string,
) (*models.ServiceCategory, error) {
	return nil, nil
}

func (r *serviceCategoryTestRepository) GetByCode(
	context.Context,
	string,
) (*models.ServiceCategory, error) {
	return nil, nil
}

func (r *serviceCategoryTestRepository) List(
	_ context.Context,
	activeOnly bool,
) ([]models.ServiceCategory, error) {
	r.gotActiveOnly = activeOnly

	if r.err != nil {
		return nil, r.err
	}

	return r.categories, nil
}

func TestServiceCategoryHandlerListActive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &serviceCategoryTestRepository{
		categories: []models.ServiceCategory{
			{
				Code:     "BASIC",
				Name:     "Basic",
				IsActive: true,
			},
		},
	}

	handler := NewServiceCategoryHandler(repo)

	router := gin.New()
	router.GET("/service-categories", handler.ListActive)

	request := httptest.NewRequest(
		http.MethodGet,
		"/service-categories",
		nil,
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusOK,
			response.Code,
		)
	}

	if !repo.gotActiveOnly {
		t.Fatal("expected service categories to be listed active-only")
	}
}

func TestServiceCategoryHandlerListActiveRepositoryError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &serviceCategoryTestRepository{
		err: errors.New("database unavailable"),
	}

	handler := NewServiceCategoryHandler(repo)

	router := gin.New()
	router.GET("/service-categories", handler.ListActive)

	request := httptest.NewRequest(
		http.MethodGet,
		"/service-categories",
		nil,
	)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusInternalServerError,
			response.Code,
		)
	}

	if !repo.gotActiveOnly {
		t.Fatal("expected service categories to be listed active-only")
	}
}
