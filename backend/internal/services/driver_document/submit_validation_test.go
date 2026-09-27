package driver_document

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func TestBuildPendingDocument(t *testing.T) {
	now := time.Date(
		2026,
		time.September,
		27,
		15,
		30,
		0,
		0,
		time.UTC,
	)

	futureExpiry := now.AddDate(1, 0, 0)

	validRequest := SubmitDocumentRequest{
		DocumentType:  models.DriverDocumentTypeTaxiDriverLicense,
		FileName:      "taxi-driver-license.pdf",
		StorageKey:    "drivers/test/taxi-driver-license.pdf",
		ContentType:   "application/pdf",
		FileSizeBytes: 4096,
		ExpiresAt:     &futureExpiry,
	}

	t.Run("valid document", func(t *testing.T) {
		document, err := buildPendingDocument(
			" driver-123 ",
			validRequest,
			now,
		)
		if err != nil {
			t.Fatalf(
				"build valid pending document: %v",
				err,
			)
		}

		if document == nil {
			t.Fatal("expected document")
		}

		if document.DriverID != "driver-123" {
			t.Fatalf(
				"expected trimmed driver ID, got %q",
				document.DriverID,
			)
		}

		if document.DocumentType !=
			models.DriverDocumentTypeTaxiDriverLicense {
			t.Fatalf(
				"expected document type %s, got %s",
				models.DriverDocumentTypeTaxiDriverLicense,
				document.DocumentType,
			)
		}

		if document.Status !=
			models.DriverDocumentStatusPending {
			t.Fatalf(
				"expected status %s, got %s",
				models.DriverDocumentStatusPending,
				document.Status,
			)
		}

		if document.VerifiedAt != nil {
			t.Fatal("expected verified_at to be nil")
		}

		if document.VerifiedByUserID != nil {
			t.Fatal("expected verified_by_user_id to be nil")
		}

		if document.RejectionReason != nil {
			t.Fatal("expected rejection_reason to be nil")
		}

		if document.ExpiresAt == nil {
			t.Fatal("expected expiry date")
		}

		if document.ExpiresAt.Hour() != 0 ||
			document.ExpiresAt.Minute() != 0 ||
			document.ExpiresAt.Second() != 0 ||
			document.ExpiresAt.Nanosecond() != 0 {
			t.Fatalf(
				"expected date-only expiry, got %v",
				document.ExpiresAt,
			)
		}
	})

	tests := []struct {
		name     string
		driverID string
		mutate   func(*SubmitDocumentRequest)
	}{
		{
			name:     "missing driver ID",
			driverID: "   ",
		},
		{
			name:     "unsupported document type",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				req.DocumentType = "PASSPORT"
			},
		},
		{
			name:     "missing file name",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				req.FileName = "   "
			},
		},
		{
			name:     "file name exceeds 255 characters",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				req.FileName = strings.Repeat("\u00e4", 256)
			},
		},
		{
			name:     "missing storage key",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				req.StorageKey = "   "
			},
		},
		{
			name:     "missing content type",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				req.ContentType = "   "
			},
		},
		{
			name:     "content type exceeds 100 characters",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				req.ContentType = strings.Repeat("a", 101)
			},
		},
		{
			name:     "zero file size",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				req.FileSizeBytes = 0
			},
		},
		{
			name:     "negative file size",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				req.FileSizeBytes = -1
			},
		},
		{
			name:     "missing expiry",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				req.ExpiresAt = nil
			},
		},
		{
			name:     "expiry today",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				expiry := now
				req.ExpiresAt = &expiry
			},
		},
		{
			name:     "expiry in past",
			driverID: "driver-123",
			mutate: func(req *SubmitDocumentRequest) {
				expiry := now.AddDate(0, 0, -1)
				req.ExpiresAt = &expiry
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest

			if tt.mutate != nil {
				tt.mutate(&req)
			}

			document, err := buildPendingDocument(
				tt.driverID,
				req,
				now,
			)

			if !errors.Is(err, ErrInvalidDocument) {
				t.Fatalf(
					"expected ErrInvalidDocument, got %v",
					err,
				)
			}

			if document != nil {
				t.Fatalf(
					"expected nil document, got %+v",
					document,
				)
			}
		})
	}
}

func TestBuildPendingDocumentTrimsMetadata(t *testing.T) {
	now := time.Now().UTC()
	expiry := now.AddDate(1, 0, 0)

	document, err := buildPendingDocument(
		" driver-123 ",
		SubmitDocumentRequest{
			DocumentType: " " +
				models.DriverDocumentTypeDrivingLicense +
				" ",
			FileName:      " driving-license.pdf ",
			StorageKey:    " drivers/test/driving-license.pdf ",
			ContentType:   " application/pdf ",
			FileSizeBytes: 2048,
			ExpiresAt:     &expiry,
		},
		now,
	)
	if err != nil {
		t.Fatalf(
			"build pending document: %v",
			err,
		)
	}

	if document.DriverID != "driver-123" ||
		document.DocumentType !=
			models.DriverDocumentTypeDrivingLicense ||
		document.FileName != "driving-license.pdf" ||
		document.StorageKey !=
			"drivers/test/driving-license.pdf" ||
		document.ContentType != "application/pdf" {
		t.Fatalf(
			"expected trimmed metadata, got %+v",
			document,
		)
	}

	if strings.TrimSpace(document.FileName) !=
		document.FileName {
		t.Fatalf(
			"file name was not normalized: %q",
			document.FileName,
		)
	}
}

func TestBuildPendingDocumentAcceptsBoundaryLengthsAndNormalizesType(
	t *testing.T,
) {
	now := time.Date(
		2026,
		time.September,
		27,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	expiry := now.AddDate(1, 0, 0)

	fileName := strings.Repeat("\u00e4", 251) + ".pdf"
	contentType := strings.Repeat("a", 100)

	document, err := buildPendingDocument(
		"driver-123",
		SubmitDocumentRequest{
			DocumentType:  " driving_license ",
			FileName:      fileName,
			StorageKey:    "drivers/test/driving-license.pdf",
			ContentType:   contentType,
			FileSizeBytes: 4096,
			ExpiresAt:     &expiry,
		},
		now,
	)
	if err != nil {
		t.Fatalf(
			"expected boundary metadata to be accepted: %v",
			err,
		)
	}

	if document == nil {
		t.Fatal("expected document")
	}

	if document.DocumentType !=
		models.DriverDocumentTypeDrivingLicense {
		t.Fatalf(
			"expected normalized document type %s, got %s",
			models.DriverDocumentTypeDrivingLicense,
			document.DocumentType,
		)
	}

	if utf8.RuneCountInString(document.FileName) != 255 {
		t.Fatalf(
			"expected 255-character file name, got %d",
			utf8.RuneCountInString(document.FileName),
		)
	}

	if len(document.ContentType) != 100 {
		t.Fatalf(
			"expected 100-character content type, got %d",
			len(document.ContentType),
		)
	}
}
