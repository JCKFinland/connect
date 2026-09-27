package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	driverdocument "github.com/JCKFinland/connect/backend/internal/services/driver_document"
)

type driverDocumentAPIService struct {
	listForUserFunc func(
		context.Context,
		string,
	) ([]models.DriverDocument, error)

	getForUserFunc func(
		context.Context,
		string,
		string,
	) (*models.DriverDocument, error)

	listFunc func(
		context.Context,
		string,
	) ([]models.DriverDocument, error)

	getFunc func(
		context.Context,
		string,
		string,
	) (*models.DriverDocument, error)

	verifyFunc func(
		context.Context,
		string,
		string,
	) (*models.DriverDocument, error)

	rejectFunc func(
		context.Context,
		string,
		string,
		string,
	) (*models.DriverDocument, error)
}

func (s *driverDocumentAPIService) ListForUser(
	ctx context.Context,
	userID string,
) ([]models.DriverDocument, error) {
	if s.listForUserFunc == nil {
		return nil, errors.New("unexpected ListForUser call")
	}

	return s.listForUserFunc(ctx, userID)
}

func (s *driverDocumentAPIService) GetForUser(
	ctx context.Context,
	userID string,
	documentID string,
) (*models.DriverDocument, error) {
	if s.getForUserFunc == nil {
		return nil, errors.New("unexpected GetForUser call")
	}

	return s.getForUserFunc(
		ctx,
		userID,
		documentID,
	)
}

func (s *driverDocumentAPIService) List(
	ctx context.Context,
	driverID string,
) ([]models.DriverDocument, error) {
	if s.listFunc == nil {
		return nil, errors.New("unexpected List call")
	}

	return s.listFunc(ctx, driverID)
}

func (s *driverDocumentAPIService) Get(
	ctx context.Context,
	driverID string,
	documentID string,
) (*models.DriverDocument, error) {
	if s.getFunc == nil {
		return nil, errors.New("unexpected Get call")
	}

	return s.getFunc(
		ctx,
		driverID,
		documentID,
	)
}

func (s *driverDocumentAPIService) Verify(
	ctx context.Context,
	documentID string,
	reviewerUserID string,
) (*models.DriverDocument, error) {
	if s.verifyFunc == nil {
		return nil, errors.New("unexpected Verify call")
	}

	return s.verifyFunc(
		ctx,
		documentID,
		reviewerUserID,
	)
}

func (s *driverDocumentAPIService) Reject(
	ctx context.Context,
	documentID string,
	reviewerUserID string,
	reason string,
) (*models.DriverDocument, error) {
	if s.rejectFunc == nil {
		return nil, errors.New("unexpected Reject call")
	}

	return s.rejectFunc(
		ctx,
		documentID,
		reviewerUserID,
		reason,
	)
}

func newDriverDocumentAPIContext(
	t *testing.T,
	method string,
	path string,
	body string,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	var requestBody *strings.Reader

	if body == "" {
		requestBody = strings.NewReader("")
	} else {
		requestBody = strings.NewReader(body)
	}

	c.Request = httptest.NewRequest(
		method,
		path,
		requestBody,
	)

	if body != "" {
		c.Request.Header.Set(
			"Content-Type",
			"application/json",
		)
	}

	return c, recorder
}

func setDriverDocumentAPIUser(
	c *gin.Context,
	userID string,
) {
	middleware.SetCurrentUser(
		c,
		&models.User{
			BaseModel: models.BaseModel{
				ID: userID,
			},
		},
	)
}

func TestDriverDocumentListForCurrentDriverUsesAuthenticatedUserID(
	t *testing.T,
) {
	const userID = "user-123"

	called := false

	service := &driverDocumentAPIService{
		listForUserFunc: func(
			_ context.Context,
			gotUserID string,
		) ([]models.DriverDocument, error) {
			called = true

			if gotUserID != userID {
				t.Fatalf(
					"user ID mismatch: got %q want %q",
					gotUserID,
					userID,
				)
			}

			return []models.DriverDocument{}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodGet,
		"/api/v1/driver/documents",
		"",
	)

	setDriverDocumentAPIUser(c, userID)

	handler.ListForCurrentDriver(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if !called {
		t.Fatal("expected ListForUser to be called")
	}
}

func TestDriverDocumentGetForCurrentDriverUsesAuthenticatedUserID(
	t *testing.T,
) {
	const (
		userID     = "user-123"
		documentID = "document-456"
	)

	service := &driverDocumentAPIService{
		getForUserFunc: func(
			_ context.Context,
			gotUserID string,
			gotDocumentID string,
		) (*models.DriverDocument, error) {
			if gotUserID != userID {
				t.Fatalf(
					"user ID mismatch: got %q want %q",
					gotUserID,
					userID,
				)
			}

			if gotDocumentID != documentID {
				t.Fatalf(
					"document ID mismatch: got %q want %q",
					gotDocumentID,
					documentID,
				)
			}

			return &models.DriverDocument{
				BaseModel: models.BaseModel{
					ID: documentID,
				},
			}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodGet,
		"/api/v1/driver/documents/"+documentID,
		"",
	)

	c.Params = gin.Params{
		{
			Key:   "document_id",
			Value: documentID,
		},
	}

	setDriverDocumentAPIUser(c, userID)

	handler.GetForCurrentDriver(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentCurrentDriverRequiresAuthenticatedUser(
	t *testing.T,
) {
	service := &driverDocumentAPIService{}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodGet,
		"/api/v1/driver/documents",
		"",
	)

	handler.ListForCurrentDriver(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusUnauthorized,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentAdministrativeGetPreservesOwnershipScope(
	t *testing.T,
) {
	const (
		driverID   = "driver-123"
		documentID = "document-456"
	)

	service := &driverDocumentAPIService{
		getFunc: func(
			_ context.Context,
			gotDriverID string,
			gotDocumentID string,
		) (*models.DriverDocument, error) {
			if gotDriverID != driverID {
				t.Fatalf(
					"driver ID mismatch: got %q want %q",
					gotDriverID,
					driverID,
				)
			}

			if gotDocumentID != documentID {
				t.Fatalf(
					"document ID mismatch: got %q want %q",
					gotDocumentID,
					documentID,
				)
			}

			return &models.DriverDocument{
				BaseModel: models.BaseModel{
					ID: documentID,
				},
				DriverID: driverID,
			}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodGet,
		"/api/v1/drivers/"+driverID+
			"/documents/"+documentID,
		"",
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: driverID,
		},
		{
			Key:   "document_id",
			Value: documentID,
		},
	}

	handler.GetForDriver(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentVerifyUsesAuthenticatedReviewerAndOwnershipCheck(
	t *testing.T,
) {
	const (
		driverID       = "driver-123"
		documentID     = "document-456"
		reviewerUserID = "reviewer-user-789"
	)

	getCalled := false
	verifyCalled := false

	service := &driverDocumentAPIService{
		getFunc: func(
			_ context.Context,
			gotDriverID string,
			gotDocumentID string,
		) (*models.DriverDocument, error) {
			getCalled = true

			if gotDriverID != driverID {
				t.Fatalf(
					"driver ID mismatch: got %q want %q",
					gotDriverID,
					driverID,
				)
			}

			if gotDocumentID != documentID {
				t.Fatalf(
					"document ID mismatch: got %q want %q",
					gotDocumentID,
					documentID,
				)
			}

			return &models.DriverDocument{
				BaseModel: models.BaseModel{
					ID: documentID,
				},
				DriverID: driverID,
			}, nil
		},

		verifyFunc: func(
			_ context.Context,
			gotDocumentID string,
			gotReviewerUserID string,
		) (*models.DriverDocument, error) {
			verifyCalled = true

			if !getCalled {
				t.Fatal(
					"Verify must not run before ownership-aware Get",
				)
			}

			if gotDocumentID != documentID {
				t.Fatalf(
					"document ID mismatch: got %q want %q",
					gotDocumentID,
					documentID,
				)
			}

			if gotReviewerUserID != reviewerUserID {
				t.Fatalf(
					"reviewer user ID mismatch: got %q want %q",
					gotReviewerUserID,
					reviewerUserID,
				)
			}

			return &models.DriverDocument{
				BaseModel: models.BaseModel{
					ID: documentID,
				},
				DriverID: driverID,
				Status:   models.DriverDocumentStatusVerified,
			}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodPost,
		"/api/v1/drivers/"+driverID+
			"/documents/"+documentID+
			"/verify",
		"",
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: driverID,
		},
		{
			Key:   "document_id",
			Value: documentID,
		},
	}

	setDriverDocumentAPIUser(
		c,
		reviewerUserID,
	)

	handler.Verify(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if !getCalled {
		t.Fatal("expected ownership-aware Get to be called")
	}

	if !verifyCalled {
		t.Fatal("expected Verify to be called")
	}
}

func TestDriverDocumentVerifyStopsWhenOwnershipCheckFails(
	t *testing.T,
) {
	const (
		driverID       = "driver-wrong"
		documentID     = "document-456"
		reviewerUserID = "reviewer-user-789"
	)

	verifyCalled := false

	service := &driverDocumentAPIService{
		getFunc: func(
			_ context.Context,
			gotDriverID string,
			gotDocumentID string,
		) (*models.DriverDocument, error) {
			if gotDriverID != driverID {
				t.Fatalf(
					"driver ID mismatch: got %q want %q",
					gotDriverID,
					driverID,
				)
			}

			if gotDocumentID != documentID {
				t.Fatalf(
					"document ID mismatch: got %q want %q",
					gotDocumentID,
					documentID,
				)
			}

			return nil, driverdocument.ErrDocumentNotFound
		},

		verifyFunc: func(
			context.Context,
			string,
			string,
		) (*models.DriverDocument, error) {
			verifyCalled = true
			return nil, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodPost,
		"/api/v1/drivers/"+driverID+
			"/documents/"+documentID+
			"/verify",
		"",
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: driverID,
		},
		{
			Key:   "document_id",
			Value: documentID,
		},
	}

	setDriverDocumentAPIUser(
		c,
		reviewerUserID,
	)

	handler.Verify(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusNotFound,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if verifyCalled {
		t.Fatal(
			"Verify must not run when ownership-aware Get fails",
		)
	}
}

func TestDriverDocumentRejectUsesAuthenticatedReviewerAndReason(
	t *testing.T,
) {
	const (
		driverID       = "driver-123"
		documentID     = "document-456"
		reviewerUserID = "reviewer-user-789"
		reason         = "document image is unreadable"
	)

	getCalled := false

	service := &driverDocumentAPIService{
		getFunc: func(
			_ context.Context,
			gotDriverID string,
			gotDocumentID string,
		) (*models.DriverDocument, error) {
			getCalled = true

			if gotDriverID != driverID ||
				gotDocumentID != documentID {
				t.Fatalf(
					"unexpected ownership lookup: driver=%q document=%q",
					gotDriverID,
					gotDocumentID,
				)
			}

			return &models.DriverDocument{
				BaseModel: models.BaseModel{
					ID: documentID,
				},
				DriverID: driverID,
			}, nil
		},

		rejectFunc: func(
			_ context.Context,
			gotDocumentID string,
			gotReviewerUserID string,
			gotReason string,
		) (*models.DriverDocument, error) {
			if !getCalled {
				t.Fatal(
					"Reject must not run before ownership-aware Get",
				)
			}

			if gotDocumentID != documentID {
				t.Fatalf(
					"document ID mismatch: got %q want %q",
					gotDocumentID,
					documentID,
				)
			}

			if gotReviewerUserID != reviewerUserID {
				t.Fatalf(
					"reviewer user ID mismatch: got %q want %q",
					gotReviewerUserID,
					reviewerUserID,
				)
			}

			if gotReason != reason {
				t.Fatalf(
					"reason mismatch: got %q want %q",
					gotReason,
					reason,
				)
			}

			return &models.DriverDocument{
				BaseModel: models.BaseModel{
					ID: documentID,
				},
				DriverID: driverID,
				Status:   models.DriverDocumentStatusRejected,
			}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodPost,
		"/api/v1/drivers/"+driverID+
			"/documents/"+documentID+
			"/reject",
		`{"reason":"`+reason+`"}`,
	)

	c.Params = gin.Params{
		{
			Key:   "id",
			Value: driverID,
		},
		{
			Key:   "document_id",
			Value: documentID,
		},
	}

	setDriverDocumentAPIUser(
		c,
		reviewerUserID,
	)

	handler.Reject(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentRejectRejectsInvalidBody(
	t *testing.T,
) {
	service := &driverDocumentAPIService{}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodPost,
		"/api/v1/drivers/driver-123/documents/document-456/reject",
		`{}`,
	)

	setDriverDocumentAPIUser(
		c,
		"reviewer-user-789",
	)

	handler.Reject(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusBadRequest,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentErrorMapping(
	t *testing.T,
) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{
			name:       "invalid document",
			err:        driverdocument.ErrInvalidDocument,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "driver not found",
			err:        driverdocument.ErrDriverNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "document not found",
			err:        driverdocument.ErrDocumentNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "already reviewed",
			err:        driverdocument.ErrDocumentAlreadyReviewed,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "unexpected internal error",
			err:        errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				service := &driverDocumentAPIService{
					listFunc: func(
						context.Context,
						string,
					) ([]models.DriverDocument, error) {
						return nil, tt.err
					},
				}

				handler := NewDriverDocumentHandler(service)

				c, recorder := newDriverDocumentAPIContext(
					t,
					http.MethodGet,
					"/api/v1/drivers/driver-123/documents",
					"",
				)

				c.Params = gin.Params{
					{
						Key:   "id",
						Value: "driver-123",
					},
				}

				handler.ListForDriver(c)

				if recorder.Code != tt.wantStatus {
					t.Fatalf(
						"expected HTTP %d, got %d: %s",
						tt.wantStatus,
						recorder.Code,
						recorder.Body.String(),
					)
				}
			},
		)
	}
}
