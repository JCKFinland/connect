package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	driverdocument "github.com/JCKFinland/connect/backend/internal/services/driver_document"
)

type driverDocumentAPIService struct {
	uploadForUserFunc func(
		context.Context,
		string,
		driverdocument.UploadDocumentRequest,
	) (*models.DriverDocument, error)

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

	revokeFunc func(
		context.Context,
		string,
		string,
		string,
	) (*models.DriverDocument, error)

	openForUserFunc func(
		context.Context,
		string,
		string,
	) (*driverdocument.OpenDocument, error)

	openFunc func(
		context.Context,
		string,
		string,
	) (*driverdocument.OpenDocument, error)
}

func (s *driverDocumentAPIService) UploadForUser(
	ctx context.Context,
	userID string,
	req driverdocument.UploadDocumentRequest,
) (*models.DriverDocument, error) {
	if s.uploadForUserFunc == nil {
		return nil, errors.New("unexpected UploadForUser call")
	}

	return s.uploadForUserFunc(
		ctx,
		userID,
		req,
	)
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

func (s *driverDocumentAPIService) OpenForUser(
	ctx context.Context,
	userID string,
	documentID string,
) (*driverdocument.OpenDocument, error) {
	if s.openForUserFunc == nil {
		return nil, errors.New(
			"unexpected OpenForUser call",
		)
	}

	return s.openForUserFunc(
		ctx,
		userID,
		documentID,
	)
}

func (s *driverDocumentAPIService) Open(
	ctx context.Context,
	driverID string,
	documentID string,
) (*driverdocument.OpenDocument, error) {
	if s.openFunc == nil {
		return nil, errors.New(
			"unexpected Open call",
		)
	}

	return s.openFunc(
		ctx,
		driverID,
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

func (s *driverDocumentAPIService) Revoke(
	ctx context.Context,
	documentID string,
	reviewerUserID string,
	reason string,
) (*models.DriverDocument, error) {
	if s.revokeFunc == nil {
		return nil, errors.New("unexpected Revoke call")
	}

	return s.revokeFunc(
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

func TestDriverDocumentUploadForCurrentDriverUsesAuthenticatedUserAndMultipartFile(
	t *testing.T,
) {
	const (
		userID       = "user-123"
		documentType = models.DriverDocumentTypeDrivingLicense
		fileName     = "client-license.exe"
	)

	expectedExpiry := time.Date(
		2030,
		time.January,
		2,
		0,
		0,
		0,
		0,
		time.UTC,
	)

	fileBytes := []byte(
		"%PDF-1.7\nCONNECT regulatory document",
	)

	called := false

	service := &driverDocumentAPIService{
		uploadForUserFunc: func(
			_ context.Context,
			gotUserID string,
			req driverdocument.UploadDocumentRequest,
		) (*models.DriverDocument, error) {
			called = true

			if gotUserID != userID {
				t.Fatalf(
					"user ID mismatch: got %q want %q",
					gotUserID,
					userID,
				)
			}

			if req.DocumentType != documentType {
				t.Fatalf(
					"document type mismatch: got %q want %q",
					req.DocumentType,
					documentType,
				)
			}

			if req.FileName != fileName {
				t.Fatalf(
					"filename mismatch: got %q want %q",
					req.FileName,
					fileName,
				)
			}

			if req.ExpiresAt == nil ||
				!req.ExpiresAt.Equal(expectedExpiry) {
				t.Fatalf(
					"expiry mismatch: got %v want %v",
					req.ExpiresAt,
					expectedExpiry,
				)
			}

			if req.Body == nil {
				t.Fatal("expected uploaded file body")
			}

			var gotBody bytes.Buffer
			if _, err := gotBody.ReadFrom(req.Body); err != nil {
				t.Fatalf(
					"read upload body: %v",
					err,
				)
			}

			if !bytes.Equal(
				gotBody.Bytes(),
				fileBytes,
			) {
				t.Fatalf(
					"file body mismatch: got %q want %q",
					gotBody.Bytes(),
					fileBytes,
				)
			}

			return &models.DriverDocument{
				BaseModel: models.BaseModel{
					ID: "document-456",
				},
				DocumentType: documentType,
				FileName:     fileName,
				Status:       models.DriverDocumentStatusPending,
			}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentMultipartAPIContext(
		t,
		map[string]string{
			"document_type": documentType,
			"expires_at":    "2030-01-02",
		},
		"file",
		fileName,
		fileBytes,
	)

	setDriverDocumentAPIUser(c, userID)

	handler.UploadForCurrentDriver(c)

	if recorder.Code != http.StatusCreated {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusCreated,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if !called {
		t.Fatal("expected UploadForUser to be called")
	}
}

func TestDriverDocumentUploadForCurrentDriverRequiresAuthenticatedUser(
	t *testing.T,
) {
	service := &driverDocumentAPIService{}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentMultipartAPIContext(
		t,
		map[string]string{
			"document_type": models.DriverDocumentTypeDrivingLicense,
			"expires_at":    "2030-01-02",
		},
		"file",
		"license.pdf",
		[]byte("%PDF-1.7\nCONNECT"),
	)

	handler.UploadForCurrentDriver(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusUnauthorized,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentUploadForCurrentDriverRejectsInvalidExpiry(
	t *testing.T,
) {
	service := &driverDocumentAPIService{}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentMultipartAPIContext(
		t,
		map[string]string{
			"document_type": models.DriverDocumentTypeDrivingLicense,
			"expires_at":    "02-01-2030",
		},
		"file",
		"license.pdf",
		[]byte("%PDF-1.7\nCONNECT"),
	)

	setDriverDocumentAPIUser(c, "user-123")

	handler.UploadForCurrentDriver(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusBadRequest,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentUploadForCurrentDriverRequiresFile(
	t *testing.T,
) {
	service := &driverDocumentAPIService{}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentMultipartAPIContext(
		t,
		map[string]string{
			"document_type": models.DriverDocumentTypeDrivingLicense,
			"expires_at":    "2030-01-02",
		},
		"",
		"",
		nil,
	)

	setDriverDocumentAPIUser(c, "user-123")

	handler.UploadForCurrentDriver(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusBadRequest,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentUploadForCurrentDriverRejectsOversizedMultipartRequest(
	t *testing.T,
) {
	called := false

	service := &driverDocumentAPIService{
		uploadForUserFunc: func(
			context.Context,
			string,
			driverdocument.UploadDocumentRequest,
		) (*models.DriverDocument, error) {
			called = true
			return nil, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	oversizedFile := bytes.Repeat(
		[]byte{'A'},
		int(driverDocumentUploadRequestMaxBytes),
	)

	c, recorder := newDriverDocumentMultipartAPIContext(
		t,
		map[string]string{
			"document_type": models.DriverDocumentTypeDrivingLicense,
			"expires_at":    "2030-01-02",
		},
		"file",
		"license.pdf",
		oversizedFile,
	)

	setDriverDocumentAPIUser(c, "user-123")

	handler.UploadForCurrentDriver(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusBadRequest,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if called {
		t.Fatal(
			"UploadForUser must not be called for oversized multipart request",
		)
	}
}

func newDriverDocumentMultipartAPIContext(
	t *testing.T,
	fields map[string]string,
	fileField string,
	fileName string,
	fileBytes []byte,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range fields {
		if err := writer.WriteField(
			key,
			value,
		); err != nil {
			t.Fatalf(
				"write multipart field %q: %v",
				key,
				err,
			)
		}
	}

	if fileField != "" {
		part, err := writer.CreateFormFile(
			fileField,
			fileName,
		)
		if err != nil {
			t.Fatalf(
				"create multipart file: %v",
				err,
			)
		}

		if _, err := part.Write(fileBytes); err != nil {
			t.Fatalf(
				"write multipart file: %v",
				err,
			)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf(
			"close multipart writer: %v",
			err,
		)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/driver/documents",
		&body,
	)

	c.Request.Header.Set(
		"Content-Type",
		writer.FormDataContentType(),
	)

	return c, recorder
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
			name:       "not revocable",
			err:        driverdocument.ErrDocumentNotRevocable,
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

func TestDriverDocumentDownloadForCurrentDriverStreamsAuthorizedBinary(
	t *testing.T,
) {
	const (
		userID           = "user-123"
		documentID       = "document-456"
		fileName         = `driver "license"; final.exe`
		downloadFileName = `driver "license"; final.pdf`
		contentType      = "application/pdf"
	)

	fileBytes := []byte("%PDF-1.7\nCONNECT regulatory document")

	service := &driverDocumentAPIService{
		openForUserFunc: func(
			_ context.Context,
			gotUserID string,
			gotDocumentID string,
		) (*driverdocument.OpenDocument, error) {
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

			return &driverdocument.OpenDocument{
				Document: &models.DriverDocument{
					BaseModel: models.BaseModel{
						ID: documentID,
					},
					FileName:      fileName,
					ContentType:   contentType,
					FileSizeBytes: int64(len(fileBytes)),
				},
				Body: io.NopCloser(
					bytes.NewReader(fileBytes),
				),
			}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodGet,
		"/api/v1/driver/documents/"+
			documentID+
			"/download",
		"",
	)

	c.Params = gin.Params{
		{
			Key:   "document_id",
			Value: documentID,
		},
	}

	setDriverDocumentAPIUser(c, userID)

	handler.DownloadForCurrentDriver(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if recorder.Header().Get("Content-Type") != contentType {
		t.Fatalf(
			"content type mismatch: got %q want %q",
			recorder.Header().Get("Content-Type"),
			contentType,
		)
	}

	contentDisposition := recorder.Header().Get(
		"Content-Disposition",
	)
	if contentDisposition == "" {
		t.Fatal("expected Content-Disposition header")
	}

	_, params, err := mime.ParseMediaType(
		contentDisposition,
	)
	if err != nil {
		t.Fatalf(
			"parse Content-Disposition: %v",
			err,
		)
	}

	if params["filename"] != downloadFileName {
		t.Fatalf(
			"download filename mismatch: got %q want %q",
			params["filename"],
			downloadFileName,
		)
	}

	if got := recorder.Header().Get(
		"X-Content-Type-Options",
	); got != "nosniff" {
		t.Fatalf(
			"X-Content-Type-Options mismatch: got %q want %q",
			got,
			"nosniff",
		)
	}

	if !bytes.Equal(
		recorder.Body.Bytes(),
		fileBytes,
	) {
		t.Fatalf(
			"binary body mismatch: got %q want %q",
			recorder.Body.Bytes(),
			fileBytes,
		)
	}
}

func TestDriverDocumentDownloadForCurrentDriverRequiresAuthenticatedUser(
	t *testing.T,
) {
	service := &driverDocumentAPIService{}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodGet,
		"/api/v1/driver/documents/document-456/download",
		"",
	)

	c.Params = gin.Params{
		{
			Key:   "document_id",
			Value: "document-456",
		},
	}

	handler.DownloadForCurrentDriver(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusUnauthorized,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentAdministrativeDownloadPreservesOwnershipScope(
	t *testing.T,
) {
	const (
		driverID   = "driver-123"
		documentID = "document-456"
	)

	fileBytes := []byte{
		0x89, 'P', 'N', 'G',
		'\r', '\n', 0x1a, '\n',
	}

	service := &driverDocumentAPIService{
		openFunc: func(
			_ context.Context,
			gotDriverID string,
			gotDocumentID string,
		) (*driverdocument.OpenDocument, error) {
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

			return &driverdocument.OpenDocument{
				Document: &models.DriverDocument{
					BaseModel: models.BaseModel{
						ID: documentID,
					},
					DriverID:      driverID,
					FileName:      "license.png",
					ContentType:   "image/png",
					FileSizeBytes: int64(len(fileBytes)),
				},
				Body: io.NopCloser(
					bytes.NewReader(fileBytes),
				),
			}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodGet,
		"/api/v1/drivers/"+driverID+
			"/documents/"+documentID+
			"/download",
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

	handler.DownloadForDriver(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusOK,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if !bytes.Equal(
		recorder.Body.Bytes(),
		fileBytes,
	) {
		t.Fatalf(
			"binary body mismatch: got %v want %v",
			recorder.Body.Bytes(),
			fileBytes,
		)
	}
}

func TestDriverDocumentDownloadMapsDocumentNotFound(
	t *testing.T,
) {
	service := &driverDocumentAPIService{
		openForUserFunc: func(
			context.Context,
			string,
			string,
		) (*driverdocument.OpenDocument, error) {
			return nil, driverdocument.ErrDocumentNotFound
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodGet,
		"/api/v1/driver/documents/document-456/download",
		"",
	)

	c.Params = gin.Params{
		{
			Key:   "document_id",
			Value: "document-456",
		},
	}

	setDriverDocumentAPIUser(c, "user-123")

	handler.DownloadForCurrentDriver(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusNotFound,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentRevokeUsesAuthenticatedReviewerAndReason(
	t *testing.T,
) {
	const (
		driverID       = "driver-123"
		documentID     = "document-456"
		reviewerUserID = "reviewer-user-789"
		reason         = "regulatory credential withdrawn"
	)

	getCalled := false
	revokeCalled := false

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
				Status:   models.DriverDocumentStatusVerified,
			}, nil
		},

		revokeFunc: func(
			_ context.Context,
			gotDocumentID string,
			gotReviewerUserID string,
			gotReason string,
		) (*models.DriverDocument, error) {
			revokeCalled = true

			if !getCalled {
				t.Fatal(
					"Revoke must not run before ownership-aware Get",
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
				Status:   models.DriverDocumentStatusRevoked,
			}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodPost,
		"/api/v1/drivers/"+driverID+
			"/documents/"+documentID+
			"/revoke",
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

	handler.Revoke(c)

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

	if !revokeCalled {
		t.Fatal("expected Revoke to be called")
	}
}

func TestDriverDocumentRevokeStopsWhenOwnershipCheckFails(
	t *testing.T,
) {
	const (
		driverID       = "driver-wrong"
		documentID     = "document-456"
		reviewerUserID = "reviewer-user-789"
	)

	revokeCalled := false

	service := &driverDocumentAPIService{
		getFunc: func(
			_ context.Context,
			gotDriverID string,
			gotDocumentID string,
		) (*models.DriverDocument, error) {
			if gotDriverID != driverID ||
				gotDocumentID != documentID {

				t.Fatalf(
					"unexpected ownership lookup: driver=%q document=%q",
					gotDriverID,
					gotDocumentID,
				)
			}

			return nil, driverdocument.ErrDocumentNotFound
		},

		revokeFunc: func(
			context.Context,
			string,
			string,
			string,
		) (*models.DriverDocument, error) {
			revokeCalled = true
			return nil, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodPost,
		"/api/v1/drivers/"+driverID+
			"/documents/"+documentID+
			"/revoke",
		`{"reason":"credential withdrawn"}`,
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

	handler.Revoke(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusNotFound,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if revokeCalled {
		t.Fatal(
			"Revoke must not run when ownership-aware Get fails",
		)
	}
}

func TestDriverDocumentRevokeRejectsInvalidBody(
	t *testing.T,
) {
	service := &driverDocumentAPIService{}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodPost,
		"/api/v1/drivers/driver-123/documents/document-456/revoke",
		`{}`,
	)

	setDriverDocumentAPIUser(
		c,
		"reviewer-user-789",
	)

	handler.Revoke(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusBadRequest,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentRevokeMapsNotRevocableToConflict(
	t *testing.T,
) {
	const (
		driverID   = "driver-123"
		documentID = "document-456"
	)

	service := &driverDocumentAPIService{
		getFunc: func(
			context.Context,
			string,
			string,
		) (*models.DriverDocument, error) {
			return &models.DriverDocument{
				BaseModel: models.BaseModel{
					ID: documentID,
				},
				DriverID: driverID,
				Status:   models.DriverDocumentStatusPending,
			}, nil
		},

		revokeFunc: func(
			context.Context,
			string,
			string,
			string,
		) (*models.DriverDocument, error) {
			return nil, driverdocument.ErrDocumentNotRevocable
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodPost,
		"/api/v1/drivers/"+driverID+
			"/documents/"+documentID+
			"/revoke",
		`{"reason":"credential withdrawn"}`,
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
		"reviewer-user-789",
	)

	handler.Revoke(c)

	if recorder.Code != http.StatusConflict {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusConflict,
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

func TestDriverDocumentDownloadFileNameUsesCanonicalExtension(
	t *testing.T,
) {
	tests := []struct {
		name        string
		fileName    string
		contentType string
		want        string
		wantOK      bool
	}{
		{
			name:        "PDF replaces misleading extension",
			fileName:    "driver-license.exe",
			contentType: "application/pdf",
			want:        "driver-license.pdf",
			wantOK:      true,
		},
		{
			name:        "JPEG replaces misleading extension",
			fileName:    "driver-license.pdf",
			contentType: "image/jpeg",
			want:        "driver-license.jpg",
			wantOK:      true,
		},
		{
			name:        "PNG replaces misleading extension",
			fileName:    "driver-license.jpg",
			contentType: "image/png",
			want:        "driver-license.png",
			wantOK:      true,
		},
		{
			name:        "missing basename uses safe fallback",
			fileName:    ".exe",
			contentType: "application/pdf",
			want:        "driver-document.pdf",
			wantOK:      true,
		},
		{
			name:        "unsupported content type fails closed",
			fileName:    "driver-license.exe",
			contentType: "application/octet-stream",
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				got, ok := driverDocumentDownloadFileName(
					tt.fileName,
					tt.contentType,
				)

				if ok != tt.wantOK {
					t.Fatalf(
						"ok = %v, want %v",
						ok,
						tt.wantOK,
					)
				}

				if got != tt.want {
					t.Fatalf(
						"filename = %q, want %q",
						got,
						tt.want,
					)
				}
			},
		)
	}
}

func TestDriverDocumentDownloadFailsClosedForUnsupportedContentType(
	t *testing.T,
) {
	const (
		userID     = "user-123"
		documentID = "document-456"
	)

	fileBytes := []byte("untrusted binary content")

	service := &driverDocumentAPIService{
		openForUserFunc: func(
			_ context.Context,
			gotUserID string,
			gotDocumentID string,
		) (*driverdocument.OpenDocument, error) {
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

			return &driverdocument.OpenDocument{
				Document: &models.DriverDocument{
					BaseModel: models.BaseModel{
						ID: documentID,
					},
					FileName:      "credential.exe",
					ContentType:   "application/octet-stream",
					FileSizeBytes: int64(len(fileBytes)),
				},
				Body: io.NopCloser(
					bytes.NewReader(fileBytes),
				),
			}, nil
		},
	}

	handler := NewDriverDocumentHandler(service)

	c, recorder := newDriverDocumentAPIContext(
		t,
		http.MethodGet,
		"/api/v1/driver/documents/"+
			documentID+
			"/download",
		"",
	)

	c.Params = gin.Params{
		{
			Key:   "document_id",
			Value: documentID,
		},
	}

	setDriverDocumentAPIUser(c, userID)

	handler.DownloadForCurrentDriver(c)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf(
			"expected HTTP %d, got %d: %s",
			http.StatusInternalServerError,
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if got := recorder.Header().Get(
		"Content-Disposition",
	); got != "" {
		t.Fatalf(
			"unexpected Content-Disposition: %q",
			got,
		)
	}

	if bytes.Contains(
		recorder.Body.Bytes(),
		fileBytes,
	) {
		t.Fatal(
			"unsupported document binary must not be streamed",
		)
	}
}
