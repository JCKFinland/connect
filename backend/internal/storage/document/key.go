package document

import (
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
)

// NewDriverDocumentKey generates an opaque, server-controlled storage key.
//
// The key intentionally contains the driver ID and document type only as
// namespace components. The original client filename is not used as a path
// component, preventing path traversal and filesystem portability problems.
func NewDriverDocumentKey(
	driverID string,
	documentType string,
	extension string,
) (string, error) {
	driverID = strings.TrimSpace(driverID)
	documentType = strings.ToLower(
		strings.TrimSpace(documentType),
	)
	extension = strings.ToLower(
		strings.TrimSpace(extension),
	)

	if driverID == "" || documentType == "" {
		return "", ErrInvalidKey
	}

	if !safeKeySegment(driverID) ||
		!safeKeySegment(documentType) {
		return "", ErrInvalidKey
	}

	if extension != "" {
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}

		if !safeExtension(extension) {
			return "", ErrInvalidKey
		}
	}

	return path.Join(
		"drivers",
		driverID,
		documentType,
		uuid.NewString()+extension,
	), nil
}

func ValidateKey(key string) error {
	key = strings.TrimSpace(key)

	if key == "" {
		return ErrInvalidKey
	}

	if strings.ContainsRune(key, '\x00') {
		return ErrInvalidKey
	}

	if strings.Contains(key, "\\") {
		return ErrInvalidKey
	}

	if strings.HasPrefix(key, "/") {
		return ErrInvalidKey
	}

	cleaned := path.Clean(key)

	if cleaned == "." ||
		cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") ||
		cleaned != key {
		return ErrInvalidKey
	}

	for _, segment := range strings.Split(cleaned, "/") {
		if !safeKeySegment(segment) {
			return ErrInvalidKey
		}
	}

	return nil
}

func safeKeySegment(value string) bool {
	if value == "" ||
		value == "." ||
		value == ".." {
		return false
	}

	if strings.ContainsAny(value, `/\`+"\x00") {
		return false
	}

	return true
}

func safeExtension(extension string) bool {
	if len(extension) < 2 ||
		len(extension) > 10 ||
		extension[0] != '.' {
		return false
	}

	for _, r := range extension[1:] {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}

	return true
}

// WrapInvalidKey adds context while preserving errors.Is(err, ErrInvalidKey).
func WrapInvalidKey(message string) error {
	return fmt.Errorf(
		"%w: %s",
		ErrInvalidKey,
		message,
	)
}
