// Package filestore keeps asset bytes in a directory, content-addressed by
// SHA-256, so a file is written once and never changed. It implements
// core/assets.Store for ADX_ASSETS_DIR.
package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/WokeWorks/adx/server/core/assets"
)

// MaxSize caps one asset. Renders are a few megabytes; brand fonts and
// videos stay under this.
const MaxSize = 100 << 20

var keyPattern = regexp.MustCompile(`^sha256-[0-9a-f]{64}$`)

type Store struct{ dir string }

// New uses dir, which must exist (cmd/adx checks storage before starting).
func New(dir string) (*Store, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("asset store: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("asset store: %s is not a directory", dir)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Put(ctx context.Context, contentType string, r io.Reader) (assets.Ref, error) {
	if contentType == "" || strings.ContainsAny(contentType, "\r\n") {
		return assets.Ref{}, errors.New("asset content type is required")
	}
	tmp, err := os.CreateTemp(s.dir, ".incoming-*")
	if err != nil {
		return assets.Ref{}, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, MaxSize+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return assets.Ref{}, err
	}
	if n > MaxSize {
		return assets.Ref{}, errors.New("asset is larger than the 100 MB limit")
	}
	if n == 0 {
		return assets.Ref{}, errors.New("asset is empty")
	}
	sum := hex.EncodeToString(h.Sum(nil))
	ref := assets.Ref{Key: "sha256-" + sum, SHA256: sum, Size: n, ContentType: contentType}
	final := s.path(ref.Key)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return assets.Ref{}, err
	}
	// Link into place only if nothing is there: identical bytes already
	// stored are left alone, so an existing file is never rewritten.
	if err := os.Link(tmp.Name(), final); err != nil && !errors.Is(err, fs.ErrExist) {
		return assets.Ref{}, err
	}
	_ = ctx
	return ref, nil
}

func (s *Store) Open(_ context.Context, ref assets.Ref) (io.ReadCloser, error) {
	if !keyPattern.MatchString(ref.Key) {
		return nil, errors.New("asset key is invalid")
	}
	f, err := os.Open(s.path(ref.Key))
	if err != nil {
		return nil, err
	}
	// Verify before serving: a corrupted or tampered file is an error, not
	// an image someone might approve.
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		f.Close()
		return nil, err
	}
	if hex.EncodeToString(h.Sum(nil)) != ref.SHA256 {
		f.Close()
		return nil, fmt.Errorf("asset %s does not match its checksum", ref.Key)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (s *Store) Exists(_ context.Context, ref assets.Ref) (bool, error) {
	if !keyPattern.MatchString(ref.Key) {
		return false, nil
	}
	_, err := os.Stat(s.path(ref.Key))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// path spreads files over 256 directories by the first two hex digits.
func (s *Store) path(key string) string {
	sum := strings.TrimPrefix(key, "sha256-")
	return filepath.Join(s.dir, "blobs", sum[:2], sum)
}
