// Package blob implements domain.BlobStore.
package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/chenhg5/myco/internal/domain"
)

var idPattern = regexp.MustCompile(`^[a-z]+_[a-z0-9]+$`)

// FS stores blobs as files in a directory (fanned out by id prefix).
type FS struct{ Dir string }

var _ domain.BlobStore = FS{}

func (s FS) path(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", domain.Errf(domain.CodeInvalidArgument, "invalid file id %q", id)
	}
	shard := id[len(id)-2:]
	return filepath.Join(s.Dir, shard, id), nil
}

func (s FS) Put(ctx context.Context, id string, r io.Reader, max int64) (int64, string, error) {
	p, err := s.path(id)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return 0, "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return 0, "", err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, max+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, "", fmt.Errorf("storing upload: %w", err)
	}
	if n > max {
		return 0, "", domain.Errf(domain.CodeInvalidArgument, "file larger than %d bytes", max).
			WithSuggestion("split it, or ask the hub admin to raise --max-file-size")
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func (s FS) Open(ctx context.Context, id string) (io.ReadCloser, error) {
	p, err := s.path(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, domain.Errf(domain.CodeNotFound, "file %s content not found", id)
	}
	return f, err
}

func (s FS) Delete(ctx context.Context, id string) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
