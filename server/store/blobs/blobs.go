// Package blobs keeps uploaded files on disk, named by their SHA-256, next to
// the database so a backup of the data folder carries both. The same bytes
// are stored once however often they're uploaded.
package blobs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// ErrTooLarge means the upload went over the limit passed to Put.
var ErrTooLarge = errors.New("file is too large")

var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Store struct {
	dir string
}

// New returns a store rooted at dir, creating it if needed.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Put copies r to disk and returns its SHA-256 and size. Reading stops one
// byte past limit, so an oversized upload fails without filling the disk.
func (s *Store) Put(r io.Reader, limit int64) (sum string, size int64, err error) {
	tmp, err := os.CreateTemp(s.dir, "upload-*")
	if err != nil {
		return "", 0, err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	size, err = io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, limit+1))
	if err != nil {
		return "", 0, err
	}
	if size > limit {
		return "", 0, ErrTooLarge
	}
	if err = tmp.Close(); err != nil {
		return "", 0, err
	}
	sum = hex.EncodeToString(h.Sum(nil))
	dest := s.path(sum)
	if err = os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", 0, err
	}
	// Renaming over an existing copy is harmless: the bytes are the same.
	if err = os.Rename(tmp.Name(), dest); err != nil {
		return "", 0, err
	}
	return sum, size, nil
}

// Open returns the file stored under sum.
func (s *Store) Open(sum string) (*os.File, error) {
	if !shaPattern.MatchString(sum) {
		return nil, fmt.Errorf("%q is not a SHA-256: %w", sum, os.ErrNotExist)
	}
	return os.Open(s.path(sum))
}

func (s *Store) path(sum string) string {
	return filepath.Join(s.dir, sum[:2], sum)
}
