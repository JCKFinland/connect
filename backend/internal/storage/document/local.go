package document

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalStorage stores regulatory-document binaries beneath a configured
// filesystem root.
//
// Storage keys always use forward-slash separators. They are validated before
// being converted to operating-system paths.
type LocalStorage struct {
	root string
}

// NewLocalStorage creates a local filesystem document store.
func NewLocalStorage(root string) (*LocalStorage, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf(
			"create local document storage: root is required",
		)
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf(
			"create local document storage: resolve root: %w",
			err,
		)
	}

	if err := os.MkdirAll(
		absoluteRoot,
		0o700,
	); err != nil {
		return nil, fmt.Errorf(
			"create local document storage: create root: %w",
			err,
		)
	}

	return &LocalStorage{
		root: filepath.Clean(absoluteRoot),
	}, nil
}

// Put atomically stores a document binary.
//
// The destination is not made visible until the entire input stream has been
// copied successfully. Failed or cancelled writes remove their temporary file.
func (s *LocalStorage) Put(
	ctx context.Context,
	req PutRequest,
) error {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return fmt.Errorf(
			"put document object: local storage is not configured",
		)
	}

	if req.Body == nil {
		return fmt.Errorf(
			"put document object: body is required",
		)
	}

	if err := ValidateKey(req.Key); err != nil {
		return fmt.Errorf(
			"put document object: %w",
			err,
		)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf(
			"put document object: %w",
			err,
		)
	}

	target, err := s.pathForKey(req.Key)
	if err != nil {
		return fmt.Errorf(
			"put document object: %w",
			err,
		)
	}

	parent := filepath.Dir(target)

	if err := os.MkdirAll(
		parent,
		0o700,
	); err != nil {
		return fmt.Errorf(
			"put document object: create parent directory: %w",
			err,
		)
	}

	temp, err := os.CreateTemp(
		parent,
		".upload-*",
	)
	if err != nil {
		return fmt.Errorf(
			"put document object: create temporary file: %w",
			err,
		)
	}

	tempName := temp.Name()
	committed := false

	defer func() {
		_ = temp.Close()

		if !committed {
			_ = os.Remove(tempName)
		}
	}()

	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf(
			"put document object: set file permissions: %w",
			err,
		)
	}

	if _, err := io.Copy(
		temp,
		&contextReader{
			ctx:    ctx,
			reader: req.Body,
		},
	); err != nil {
		return fmt.Errorf(
			"put document object: copy binary: %w",
			err,
		)
	}

	if err := temp.Sync(); err != nil {
		return fmt.Errorf(
			"put document object: sync temporary file: %w",
			err,
		)
	}

	if err := temp.Close(); err != nil {
		return fmt.Errorf(
			"put document object: close temporary file: %w",
			err,
		)
	}

	// os.Rename is atomic when source and destination are on the same
	// filesystem. The temporary file is deliberately created in the target
	// directory to preserve that guarantee.
	if err := os.Rename(
		tempName,
		target,
	); err != nil {
		return fmt.Errorf(
			"put document object: commit file: %w",
			err,
		)
	}

	committed = true

	return nil
}

func (s *LocalStorage) Open(
	ctx context.Context,
	key string,
) (io.ReadCloser, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return nil, fmt.Errorf(
			"open document object: local storage is not configured",
		)
	}

	if err := ValidateKey(key); err != nil {
		return nil, fmt.Errorf(
			"open document object: %w",
			err,
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf(
			"open document object: %w",
			err,
		)
	}

	target, err := s.pathForKey(key)
	if err != nil {
		return nil, fmt.Errorf(
			"open document object: %w",
			err,
		)
	}

	file, err := os.Open(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf(
			"open document object: %w",
			err,
		)
	}

	return file, nil
}

// Delete removes a stored document object.
//
// Deleting a missing object is reported as ErrNotFound so callers can decide
// whether absence is acceptable for their consistency workflow.
func (s *LocalStorage) Delete(
	ctx context.Context,
	key string,
) error {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return fmt.Errorf(
			"delete document object: local storage is not configured",
		)
	}

	if err := ValidateKey(key); err != nil {
		return fmt.Errorf(
			"delete document object: %w",
			err,
		)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf(
			"delete document object: %w",
			err,
		)
	}

	target, err := s.pathForKey(key)
	if err != nil {
		return fmt.Errorf(
			"delete document object: %w",
			err,
		)
	}

	if err := os.Remove(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}

		return fmt.Errorf(
			"delete document object: remove file: %w",
			err,
		)
	}

	return nil
}

func (s *LocalStorage) pathForKey(
	key string,
) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}

	target := filepath.Join(
		s.root,
		filepath.FromSlash(key),
	)

	relative, err := filepath.Rel(
		s.root,
		target,
	)
	if err != nil {
		return "", fmt.Errorf(
			"resolve document storage path: %w",
			err,
		)
	}

	if relative == "." ||
		relative == ".." ||
		filepath.IsAbs(relative) ||
		strings.HasPrefix(
			relative,
			".."+string(os.PathSeparator),
		) {
		return "", ErrInvalidKey
	}

	return target, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(
	p []byte,
) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.reader.Read(p)
}
