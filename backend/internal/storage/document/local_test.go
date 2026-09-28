package document

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStoragePutAndDelete(t *testing.T) {
	root := t.TempDir()

	storage, err := NewLocalStorage(root)
	if err != nil {
		t.Fatalf(
			"create local storage: %v",
			err,
		)
	}

	key :=
		"drivers/driver-123/driving_license/document-456.pdf"

	body := []byte("CONNECT regulatory document")

	if err := storage.Put(
		context.Background(),
		PutRequest{
			Key:  key,
			Body: bytes.NewReader(body),
		},
	); err != nil {
		t.Fatalf(
			"put document: %v",
			err,
		)
	}

	path := filepath.Join(
		root,
		filepath.FromSlash(key),
	)

	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf(
			"read stored document: %v",
			err,
		)
	}

	if !bytes.Equal(stored, body) {
		t.Fatalf(
			"stored body mismatch: got %q want %q",
			stored,
			body,
		)
	}

	if err := storage.Delete(
		context.Background(),
		key,
	); err != nil {
		t.Fatalf(
			"delete document: %v",
			err,
		)
	}

	if _, err := os.Stat(path); !errors.Is(
		err,
		os.ErrNotExist,
	) {
		t.Fatalf(
			"expected stored document to be removed, got %v",
			err,
		)
	}
}

func TestLocalStorageRejectsTraversal(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf(
			"create local storage: %v",
			err,
		)
	}

	err = storage.Put(
		context.Background(),
		PutRequest{
			Key:  "../outside.pdf",
			Body: strings.NewReader("unsafe"),
		},
	)

	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf(
			"expected ErrInvalidKey, got %v",
			err,
		)
	}
}

func TestLocalStorageDeleteMissingReturnsNotFound(
	t *testing.T,
) {
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf(
			"create local storage: %v",
			err,
		)
	}

	err = storage.Delete(
		context.Background(),
		"drivers/driver-123/driving_license/missing.pdf",
	)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf(
			"expected ErrNotFound, got %v",
			err,
		)
	}
}

func TestLocalStoragePutRejectsCancelledContext(
	t *testing.T,
) {
	root := t.TempDir()

	storage, err := NewLocalStorage(root)
	if err != nil {
		t.Fatalf(
			"create local storage: %v",
			err,
		)
	}

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	key :=
		"drivers/driver-123/driving_license/cancelled.pdf"

	err = storage.Put(
		ctx,
		PutRequest{
			Key:  key,
			Body: strings.NewReader("document"),
		},
	)

	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"expected context cancellation, got %v",
			err,
		)
	}

	path := filepath.Join(
		root,
		filepath.FromSlash(key),
	)

	if _, statErr := os.Stat(path); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf(
			"cancelled write must not create target: %v",
			statErr,
		)
	}
}

func TestLocalStorageFailedCopyLeavesNoTarget(
	t *testing.T,
) {
	root := t.TempDir()

	storage, err := NewLocalStorage(root)
	if err != nil {
		t.Fatalf(
			"create local storage: %v",
			err,
		)
	}

	key :=
		"drivers/driver-123/driving_license/failed.pdf"

	err = storage.Put(
		context.Background(),
		PutRequest{
			Key: key,
			Body: &failingReader{
				err: errors.New("controlled read failure"),
			},
		},
	)

	if err == nil {
		t.Fatal(
			"expected controlled storage failure",
		)
	}

	path := filepath.Join(
		root,
		filepath.FromSlash(key),
	)

	if _, statErr := os.Stat(path); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf(
			"failed write must not create target: %v",
			statErr,
		)
	}

	matches, globErr := filepath.Glob(
		filepath.Join(
			filepath.Dir(path),
			".upload-*",
		),
	)
	if globErr != nil {
		t.Fatalf(
			"glob temporary files: %v",
			globErr,
		)
	}

	if len(matches) != 0 {
		t.Fatalf(
			"temporary files leaked: %v",
			matches,
		)
	}
}

type failingReader struct {
	err error
}

func (r *failingReader) Read(
	_ []byte,
) (int, error) {
	return 0, r.err
}
