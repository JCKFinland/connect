package document

import (
	"errors"
	"strings"
	"testing"
)

func TestNewDriverDocumentKey(t *testing.T) {
	key, err := NewDriverDocumentKey(
		"driver-123",
		"TAXI_DRIVER_LICENSE",
		".PDF",
	)
	if err != nil {
		t.Fatalf(
			"generate key: %v",
			err,
		)
	}

	prefix :=
		"drivers/driver-123/taxi_driver_license/"

	if !strings.HasPrefix(key, prefix) {
		t.Fatalf(
			"unexpected key prefix: %q",
			key,
		)
	}

	if !strings.HasSuffix(key, ".pdf") {
		t.Fatalf(
			"expected normalized .pdf extension: %q",
			key,
		)
	}

	if err := ValidateKey(key); err != nil {
		t.Fatalf(
			"generated key must be valid: %v",
			err,
		)
	}
}

func TestNewDriverDocumentKeyRejectsUnsafeSegments(
	t *testing.T,
) {
	tests := []struct {
		name         string
		driverID     string
		documentType string
		extension    string
	}{
		{
			name:         "empty driver",
			driverID:     "",
			documentType: "DRIVING_LICENSE",
			extension:    ".pdf",
		},
		{
			name:         "driver traversal",
			driverID:     "../outside",
			documentType: "DRIVING_LICENSE",
			extension:    ".pdf",
		},
		{
			name:         "document type separator",
			driverID:     "driver-123",
			documentType: "../license",
			extension:    ".pdf",
		},
		{
			name:         "unsafe extension",
			driverID:     "driver-123",
			documentType: "DRIVING_LICENSE",
			extension:    "../exe",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				_, err := NewDriverDocumentKey(
					tt.driverID,
					tt.documentType,
					tt.extension,
				)

				if !errors.Is(
					err,
					ErrInvalidKey,
				) {
					t.Fatalf(
						"expected ErrInvalidKey, got %v",
						err,
					)
				}
			},
		)
	}
}

func TestValidateKeyRejectsUnsafeKeys(t *testing.T) {
	keys := []string{
		"",
		".",
		"..",
		"../outside.pdf",
		"drivers/../outside.pdf",
		"/absolute/file.pdf",
		`drivers\driver-123\file.pdf`,
		"drivers//file.pdf",
		"drivers/./file.pdf",
		"drivers/\x00/file.pdf",
	}

	for _, key := range keys {
		t.Run(
			key,
			func(t *testing.T) {
				err := ValidateKey(key)

				if !errors.Is(
					err,
					ErrInvalidKey,
				) {
					t.Fatalf(
						"expected ErrInvalidKey for %q, got %v",
						key,
						err,
					)
				}
			},
		)
	}
}

func TestValidateKeyAcceptsGeneratedNamespace(
	t *testing.T,
) {
	key :=
		"drivers/driver-123/driving_license/document-456.pdf"

	if err := ValidateKey(key); err != nil {
		t.Fatalf(
			"expected valid key, got %v",
			err,
		)
	}
}
