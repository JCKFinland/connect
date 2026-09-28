package driver_document

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"
)

const documentSignatureBytes = 512

type validatedDocumentBinary struct {
	Body          io.Reader
	ContentType   string
	Extension     string
	FileSizeBytes int64
}

func validateDocumentFileName(fileName string) error {
	fileName = strings.TrimSpace(fileName)

	if fileName == "" {
		return fmt.Errorf(
			"%w: document filename is required",
			ErrInvalidDocument,
		)
	}

	if fileName == "." || fileName == ".." {
		return fmt.Errorf(
			"%w: invalid document filename",
			ErrInvalidDocument,
		)
	}

	if strings.ContainsAny(fileName, `/\`) {
		return fmt.Errorf(
			"%w: document filename must not contain path separators",
			ErrInvalidDocument,
		)
	}

	for _, r := range fileName {
		if unicode.IsControl(r) {
			return fmt.Errorf(
				"%w: document filename contains control characters",
				ErrInvalidDocument,
			)
		}
	}

	return nil
}

func validateDocumentBinary(
	body io.Reader,
	maxBytes int64,
) (*validatedDocumentBinary, error) {
	if body == nil {
		return nil, fmt.Errorf(
			"%w: document body is required",
			ErrInvalidDocument,
		)
	}

	if maxBytes <= 0 {
		return nil, fmt.Errorf(
			"%w: upload size limit must be greater than zero",
			ErrInvalidDocument,
		)
	}

	// Read at most maxBytes+1 so oversized uploads are rejected without
	// buffering an unbounded request body in memory.
	limited := io.LimitReader(
		body,
		maxBytes+1,
	)

	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: read document binary: %v",
			ErrInvalidDocument,
			err,
		)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf(
			"%w: document binary is empty",
			ErrInvalidDocument,
		)
	}

	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf(
			"%w: document exceeds maximum upload size",
			ErrInvalidDocument,
		)
	}

	contentType, extension, err :=
		detectDocumentBinaryType(data)
	if err != nil {
		return nil, err
	}

	return &validatedDocumentBinary{
		Body:          bytes.NewReader(data),
		ContentType:   contentType,
		Extension:     extension,
		FileSizeBytes: int64(len(data)),
	}, nil
}

func detectDocumentBinaryType(
	data []byte,
) (string, string, error) {
	signature := data

	if len(signature) > documentSignatureBytes {
		signature =
			signature[:documentSignatureBytes]
	}

	switch {
	case bytes.HasPrefix(
		signature,
		[]byte("%PDF-"),
	):
		return "application/pdf", ".pdf", nil

	case len(signature) >= 3 &&
		signature[0] == 0xff &&
		signature[1] == 0xd8 &&
		signature[2] == 0xff:
		return "image/jpeg", ".jpg", nil

	case bytes.HasPrefix(
		signature,
		[]byte{
			0x89,
			0x50,
			0x4e,
			0x47,
			0x0d,
			0x0a,
			0x1a,
			0x0a,
		},
	):
		return "image/png", ".png", nil

	default:
		return "", "", fmt.Errorf(
			"%w: unsupported document binary type",
			ErrInvalidDocument,
		)
	}
}
