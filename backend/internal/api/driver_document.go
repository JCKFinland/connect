package api

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	driverdocument "github.com/JCKFinland/connect/backend/internal/services/driver_document"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

const driverDocumentUploadRequestMaxBytes int64 = 11 * 1024 * 1024

// DriverDocumentService defines the driver-document operations required by
// the HTTP boundary. Keeping this boundary interface-based allows the API layer
// to be tested independently from PostgreSQL.
type DriverDocumentService interface {
	UploadForUser(
		ctx context.Context,
		userID string,
		req driverdocument.UploadDocumentRequest,
	) (*models.DriverDocument, error)

	ListForUser(
		ctx context.Context,
		userID string,
	) ([]models.DriverDocument, error)

	GetForUser(
		ctx context.Context,
		userID string,
		documentID string,
	) (*models.DriverDocument, error)

	OpenForUser(
		ctx context.Context,
		userID string,
		documentID string,
	) (*driverdocument.OpenDocument, error)

	Open(
		ctx context.Context,
		driverID string,
		documentID string,
	) (*driverdocument.OpenDocument, error)

	List(
		ctx context.Context,
		driverID string,
	) ([]models.DriverDocument, error)

	Get(
		ctx context.Context,
		driverID string,
		documentID string,
	) (*models.DriverDocument, error)

	Verify(
		ctx context.Context,
		documentID string,
		reviewerUserID string,
	) (*models.DriverDocument, error)

	Reject(
		ctx context.Context,
		documentID string,
		reviewerUserID string,
		reason string,
	) (*models.DriverDocument, error)
}

// DriverDocumentHandler exposes authenticated driver-document read and
// administrative review operations.
type DriverDocumentHandler struct {
	service DriverDocumentService
}

// NewDriverDocumentHandler creates the driver-document HTTP handler.
func NewDriverDocumentHandler(
	service DriverDocumentService,
) *DriverDocumentHandler {
	return &DriverDocumentHandler{
		service: service,
	}
}

// rejectDocumentRequest contains the administrator's rejection reason.
// Reviewer identity is always derived from the authenticated user.
type rejectDocumentRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// handleDriverDocumentError maps driver-document domain errors to stable HTTP
// responses without leaking persistence or internal implementation details.
func handleDriverDocumentError(
	c *gin.Context,
	err error,
) {
	switch {
	case errors.Is(err, driverdocument.ErrInvalidDocument):
		response.BadRequest(
			c,
			"invalid driver document request",
		)

	case errors.Is(err, driverdocument.ErrDriverNotFound):
		response.NotFound(
			c,
			"driver not found",
		)

	case errors.Is(err, driverdocument.ErrDocumentNotFound):
		response.NotFound(
			c,
			"driver document not found",
		)

	case errors.Is(
		err,
		driverdocument.ErrDocumentAlreadyReviewed,
	):
		response.Conflict(
			c,
			"driver document has already been reviewed",
		)

	default:
		response.InternalServerError(c)
	}
}

func serveDriverDocumentBinary(
	c *gin.Context,
	opened *driverdocument.OpenDocument,
) {
	if opened == nil ||
		opened.Document == nil ||
		opened.Body == nil {
		response.InternalServerError(c)
		return
	}
	defer opened.Body.Close()

	c.Header(
		"Content-Type",
		opened.Document.ContentType,
	)

	c.Header(
		"Content-Disposition",
		mime.FormatMediaType(
			"attachment",
			map[string]string{
				"filename": opened.Document.FileName,
			},
		),
	)

	if opened.Document.FileSizeBytes >= 0 {
		c.Header(
			"Content-Length",
			strconv.FormatInt(
				opened.Document.FileSizeBytes,
				10,
			),
		)
	}

	c.Status(http.StatusOK)

	if _, err := io.Copy(
		c.Writer,
		opened.Body,
	); err != nil {
		_ = c.Error(err)
	}
}

// UploadForCurrentDriver handles POST /api/v1/driver/documents.
//
// The authenticated users.id is translated to the authoritative drivers.id
// inside the driver-document service. Storage keys, content type, file size,
// and storage extension are never accepted from multipart metadata.
func (h *DriverDocumentHandler) UploadForCurrentDriver(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	// Bound the complete multipart request before Gin parses form fields or
	// creates temporary multipart files. The service independently enforces
	// the stricter 10 MiB binary-document limit.
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		driverDocumentUploadRequestMaxBytes,
	)

	documentType := c.PostForm("document_type")
	expiresAtRaw := c.PostForm("expires_at")

	if documentType == "" || expiresAtRaw == "" {
		response.BadRequest(
			c,
			"document_type and expires_at are required",
		)
		return
	}

	expiresAt, err := time.Parse(
		"2006-01-02",
		expiresAtRaw,
	)
	if err != nil {
		response.BadRequest(
			c,
			"expires_at must use YYYY-MM-DD format",
		)
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(
			c,
			"document file is required",
		)
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		response.BadRequest(
			c,
			"document file could not be opened",
		)
		return
	}
	defer file.Close()

	document, err := h.service.UploadForUser(
		c.Request.Context(),
		user.ID,
		driverdocument.UploadDocumentRequest{
			DocumentType: documentType,
			FileName:     fileHeader.Filename,
			ExpiresAt:    &expiresAt,
			Body:         file,
		},
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	response.Created(
		c,
		"Driver document uploaded successfully",
		document,
	)
}

// ListForCurrentDriver handles GET /api/v1/driver/documents.
func (h *DriverDocumentHandler) ListForCurrentDriver(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	documents, err := h.service.ListForUser(
		c.Request.Context(),
		user.ID,
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	response.OK(
		c,
		"Driver documents retrieved successfully",
		documents,
	)
}

// GetForCurrentDriver handles
// GET /api/v1/driver/documents/:document_id.
func (h *DriverDocumentHandler) GetForCurrentDriver(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	document, err := h.service.GetForUser(
		c.Request.Context(),
		user.ID,
		c.Param("document_id"),
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	response.OK(
		c,
		"Driver document retrieved successfully",
		document,
	)
}

// DownloadForCurrentDriver handles
// GET /api/v1/driver/documents/:document_id/download.
//
// Ownership is derived from the authenticated users.id. Clients never supply
// a storage key or authoritative driver identity.
func (h *DriverDocumentHandler) DownloadForCurrentDriver(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	opened, err := h.service.OpenForUser(
		c.Request.Context(),
		user.ID,
		c.Param("document_id"),
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	serveDriverDocumentBinary(c, opened)
}

// ListForDriver handles GET /api/v1/drivers/:id/documents.
func (h *DriverDocumentHandler) ListForDriver(
	c *gin.Context,
) {
	documents, err := h.service.List(
		c.Request.Context(),
		c.Param("id"),
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	response.OK(
		c,
		"Driver documents retrieved successfully",
		documents,
	)
}

// GetForDriver handles
// GET /api/v1/drivers/:id/documents/:document_id.
func (h *DriverDocumentHandler) GetForDriver(
	c *gin.Context,
) {
	document, err := h.service.Get(
		c.Request.Context(),
		c.Param("id"),
		c.Param("document_id"),
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	response.OK(
		c,
		"Driver document retrieved successfully",
		document,
	)
}

// DownloadForDriver handles
// GET /api/v1/drivers/:id/documents/:document_id/download.
//
// The service verifies that document_id belongs to :id before opening the
// server-controlled storage key.
func (h *DriverDocumentHandler) DownloadForDriver(
	c *gin.Context,
) {
	opened, err := h.service.Open(
		c.Request.Context(),
		c.Param("id"),
		c.Param("document_id"),
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	serveDriverDocumentBinary(c, opened)
}

// Verify handles
// POST /api/v1/drivers/:id/documents/:document_id/verify.
//
// The :id driver parameter is intentionally checked against document ownership
// before review so an administrator cannot review a document through the wrong
// driver resource path.
func (h *DriverDocumentHandler) Verify(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	document, err := h.service.Get(
		c.Request.Context(),
		c.Param("id"),
		c.Param("document_id"),
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	document, err = h.service.Verify(
		c.Request.Context(),
		document.ID,
		user.ID,
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	response.OK(
		c,
		"Driver document verified successfully",
		document,
	)
}

// Reject handles
// POST /api/v1/drivers/:id/documents/:document_id/reject.
func (h *DriverDocumentHandler) Reject(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	var req rejectDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			"invalid request body",
		)
		return
	}

	document, err := h.service.Get(
		c.Request.Context(),
		c.Param("id"),
		c.Param("document_id"),
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	document, err = h.service.Reject(
		c.Request.Context(),
		document.ID,
		user.ID,
		req.Reason,
	)
	if err != nil {
		handleDriverDocumentError(c, err)
		return
	}

	response.OK(
		c,
		"Driver document rejected successfully",
		document,
	)
}
