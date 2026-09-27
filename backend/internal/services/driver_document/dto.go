package driver_document

import "time"

// SubmitDocumentRequest contains metadata for a driver regulatory document
// whose binary has already been accepted by the configured storage layer.
//
// DriverID is intentionally not part of this request. The driver is supplied
// separately by the trusted service/API context rather than accepted as
// document metadata from the request body.
type SubmitDocumentRequest struct {
	DocumentType string `json:"document_type" binding:"required"`

	FileName   string `json:"file_name" binding:"required"`
	StorageKey string `json:"-"`

	ContentType   string `json:"content_type" binding:"required"`
	FileSizeBytes int64  `json:"file_size_bytes" binding:"required"`

	ExpiresAt *time.Time `json:"expires_at" binding:"required"`
}
