package driver_document

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateDocumentBinaryAcceptsPDF(
	t *testing.T,
) {
	data := []byte(
		"%PDF-1.7\nCONNECT test document",
	)

	result, err := validateDocumentBinary(
		bytes.NewReader(data),
		1024,
	)
	if err != nil {
		t.Fatalf(
			"validate PDF: %v",
			err,
		)
	}

	if result.ContentType != "application/pdf" {
		t.Fatalf(
			"expected application/pdf, got %q",
			result.ContentType,
		)
	}

	if result.Extension != ".pdf" {
		t.Fatalf(
			"expected .pdf, got %q",
			result.Extension,
		)
	}

	if result.FileSizeBytes != int64(len(data)) {
		t.Fatalf(
			"expected size %d, got %d",
			len(data),
			result.FileSizeBytes,
		)
	}

	stored, err := readValidatedBinary(result)
	if err != nil {
		t.Fatalf(
			"read validated PDF: %v",
			err,
		)
	}

	if !bytes.Equal(stored, data) {
		t.Fatal(
			"validated PDF body changed",
		)
	}
}

func TestValidateDocumentBinaryAcceptsJPEG(
	t *testing.T,
) {
	data := []byte{
		0xff,
		0xd8,
		0xff,
		0xe0,
		0x00,
		0x10,
		'J',
		'F',
		'I',
		'F',
	}

	result, err := validateDocumentBinary(
		bytes.NewReader(data),
		1024,
	)
	if err != nil {
		t.Fatalf(
			"validate JPEG: %v",
			err,
		)
	}

	if result.ContentType != "image/jpeg" {
		t.Fatalf(
			"expected image/jpeg, got %q",
			result.ContentType,
		)
	}

	if result.Extension != ".jpg" {
		t.Fatalf(
			"expected .jpg, got %q",
			result.Extension,
		)
	}
}

func TestValidateDocumentBinaryAcceptsPNG(
	t *testing.T,
) {
	data := []byte{
		0x89,
		0x50,
		0x4e,
		0x47,
		0x0d,
		0x0a,
		0x1a,
		0x0a,
		0x00,
		0x00,
		0x00,
		0x0d,
	}

	result, err := validateDocumentBinary(
		bytes.NewReader(data),
		1024,
	)
	if err != nil {
		t.Fatalf(
			"validate PNG: %v",
			err,
		)
	}

	if result.ContentType != "image/png" {
		t.Fatalf(
			"expected image/png, got %q",
			result.ContentType,
		)
	}

	if result.Extension != ".png" {
		t.Fatalf(
			"expected .png, got %q",
			result.Extension,
		)
	}
}

func TestValidateDocumentBinaryRejectsUnsupportedType(
	t *testing.T,
) {
	data := []byte(
		"MZ executable content",
	)

	result, err := validateDocumentBinary(
		bytes.NewReader(data),
		1024,
	)

	if !errors.Is(
		err,
		ErrInvalidDocument,
	) {
		t.Fatalf(
			"expected ErrInvalidDocument, got %v",
			err,
		)
	}

	if result != nil {
		t.Fatalf(
			"expected nil result, got %+v",
			result,
		)
	}
}

func TestValidateDocumentBinaryRejectsEmptyBody(
	t *testing.T,
) {
	result, err := validateDocumentBinary(
		bytes.NewReader(nil),
		1024,
	)

	if !errors.Is(
		err,
		ErrInvalidDocument,
	) {
		t.Fatalf(
			"expected ErrInvalidDocument, got %v",
			err,
		)
	}

	if result != nil {
		t.Fatalf(
			"expected nil result, got %+v",
			result,
		)
	}
}

func TestValidateDocumentBinaryRejectsOversizedBody(
	t *testing.T,
) {
	data := append(
		[]byte("%PDF-1.7\n"),
		[]byte(strings.Repeat("A", 64))...,
	)

	result, err := validateDocumentBinary(
		bytes.NewReader(data),
		32,
	)

	if !errors.Is(
		err,
		ErrInvalidDocument,
	) {
		t.Fatalf(
			"expected ErrInvalidDocument, got %v",
			err,
		)
	}

	if result != nil {
		t.Fatalf(
			"expected nil result, got %+v",
			result,
		)
	}
}

func TestValidateDocumentBinaryRejectsNilBody(
	t *testing.T,
) {
	result, err := validateDocumentBinary(
		nil,
		1024,
	)

	if !errors.Is(
		err,
		ErrInvalidDocument,
	) {
		t.Fatalf(
			"expected ErrInvalidDocument, got %v",
			err,
		)
	}

	if result != nil {
		t.Fatalf(
			"expected nil result, got %+v",
			result,
		)
	}
}

func TestValidateDocumentBinaryRejectsInvalidLimit(
	t *testing.T,
) {
	result, err := validateDocumentBinary(
		bytes.NewReader(
			[]byte("%PDF-1.7"),
		),
		0,
	)

	if !errors.Is(
		err,
		ErrInvalidDocument,
	) {
		t.Fatalf(
			"expected ErrInvalidDocument, got %v",
			err,
		)
	}

	if result != nil {
		t.Fatalf(
			"expected nil result, got %+v",
			result,
		)
	}
}

func readValidatedBinary(
	result *validatedDocumentBinary,
) ([]byte, error) {
	var destination bytes.Buffer

	_, err := destination.ReadFrom(
		result.Body,
	)
	if err != nil {
		return nil, err
	}

	return destination.Bytes(), nil
}

func TestValidateDocumentFileNameAcceptsOrdinaryName(
	t *testing.T,
) {
	err := validateDocumentFileName(
		"driving-license-client-name.exe",
	)
	if err != nil {
		t.Fatalf(
			"expected filename to be accepted: %v",
			err,
		)
	}
}

func TestValidateDocumentFileNameRejectsUnsafeNames(
	t *testing.T,
) {
	tests := []string{
		"",
		"   ",
		".",
		"..",
		"../license.pdf",
		`..\license.pdf`,
		"folder/license.pdf",
		`folder\license.pdf`,
		"license\x00.pdf",
		"license\n.pdf",
		"license\r.pdf",
		"license\t.pdf",
	}

	for _, fileName := range tests {
		t.Run(
			fmt.Sprintf("%q", fileName),
			func(t *testing.T) {
				err := validateDocumentFileName(fileName)

				if !errors.Is(
					err,
					ErrInvalidDocument,
				) {
					t.Fatalf(
						"expected ErrInvalidDocument for %q, got %v",
						fileName,
						err,
					)
				}
			},
		)
	}
}
