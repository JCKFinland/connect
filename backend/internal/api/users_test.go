package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

type userHandlerRoleRepository struct {
	repository.UserRoleRepository

	roles []string
	err   error
}

func (r *userHandlerRoleRepository) GetUserRoles(
	_ context.Context,
	_ string,
) ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}

	return r.roles, nil
}

func newUserHandlerTestContext(
	t *testing.T,
	user *models.User,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)

	if user != nil {
		middleware.SetCurrentUser(c, user)
	}

	return c, recorder
}

func TestUserMeReturnsProfileAndRoles(t *testing.T) {
	roleRepo := &userHandlerRoleRepository{
		roles: []string{"DRIVER"},
	}

	handler := NewUserHandler(roleRepo)

	user := &models.User{}
	user.ID = "user-1"
	user.Email = "driver@example.com"
	user.FirstName = "Driver"
	user.LastName = "One"
	user.Phone = "+358401234567"
	user.IsActive = true
	user.IsVerified = true

	c, recorder := newUserHandlerTestContext(t, user)

	handler.Me(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body struct {
		Data struct {
			ID    string   `json:"id"`
			Email string   `json:"email"`
			Roles []string `json:"roles"`
		} `json:"data"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Data.ID != user.ID {
		t.Fatalf("expected user ID %q, got %q", user.ID, body.Data.ID)
	}

	if body.Data.Email != user.Email {
		t.Fatalf("expected email %q, got %q", user.Email, body.Data.Email)
	}

	if len(body.Data.Roles) != 1 || body.Data.Roles[0] != "DRIVER" {
		t.Fatalf("expected DRIVER role, got %#v", body.Data.Roles)
	}
}

func TestUserMeReturnsEmptyRolesArray(t *testing.T) {
	handler := NewUserHandler(
		&userHandlerRoleRepository{
			roles: nil,
		},
	)

	user := &models.User{}
	user.ID = "user-1"
	user.Email = "user@example.com"
	user.IsActive = true

	c, recorder := newUserHandlerTestContext(t, user)

	handler.Me(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var body struct {
		Data struct {
			Roles []string `json:"roles"`
		} `json:"data"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Data.Roles == nil {
		t.Fatal("expected roles to be an empty JSON array, got null")
	}

	if len(body.Data.Roles) != 0 {
		t.Fatalf("expected no roles, got %#v", body.Data.Roles)
	}
}

func TestUserMeReturnsUnauthorizedWithoutCurrentUser(t *testing.T) {
	handler := NewUserHandler(
		&userHandlerRoleRepository{},
	)

	c, recorder := newUserHandlerTestContext(t, nil)

	handler.Me(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusUnauthorized,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestUserMeReturnsInternalServerErrorWhenRolesFail(t *testing.T) {
	handler := NewUserHandler(
		&userHandlerRoleRepository{
			err: errors.New("role lookup failed"),
		},
	)

	user := &models.User{}
	user.ID = "user-1"
	user.Email = "driver@example.com"
	user.IsActive = true

	c, recorder := newUserHandlerTestContext(t, user)

	handler.Me(c)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusInternalServerError,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}
