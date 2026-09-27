package models

import "time"

const (
	DriverDocumentTypeTaxiDriverLicense = "TAXI_DRIVER_LICENSE"
	DriverDocumentTypeDrivingLicense    = "DRIVING_LICENSE"
)

const (
	DriverDocumentStatusPending  = "PENDING"
	DriverDocumentStatusVerified = "VERIFIED"
	DriverDocumentStatusRejected = "REJECTED"
)

// DriverDocument represents the metadata and verification state of a
// regulatory document submitted by a driver.
//
// The document binary is stored outside PostgreSQL. StorageKey identifies
// the object in the configured document storage implementation.
type DriverDocument struct {
	BaseModel
	SoftDelete

	DriverID string `db:"driver_id" json:"driver_id"`

	DocumentType string `db:"document_type" json:"document_type"`

	FileName      string `db:"file_name" json:"file_name"`
	StorageKey    string `db:"storage_key" json:"-"`
	ContentType   string `db:"content_type" json:"content_type"`
	FileSizeBytes int64  `db:"file_size_bytes" json:"file_size_bytes"`

	ExpiresAt *time.Time `db:"expires_at" json:"expires_at,omitempty"`

	Status string `db:"status" json:"status"`

	VerifiedAt       *time.Time `db:"verified_at" json:"verified_at,omitempty"`
	VerifiedByUserID *string    `db:"verified_by_user_id" json:"verified_by_user_id,omitempty"`

	RejectionReason *string `db:"rejection_reason" json:"rejection_reason,omitempty"`
}
