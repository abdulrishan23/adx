package filestore_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/providers/filestore"
)

// Catches: the same bytes stored twice under different keys, a stored file
// being rewritten, a tampered file served as if intact, and a key escaping
// the directory.
func TestStoreIsContentAddressedAndImmutable(t *testing.T) {
	dir := t.TempDir()
	s, err := filestore.New(dir)
	require.NoError(t, err)
	ctx := t.Context()

	a, err := s.Put(ctx, "image/jpeg", strings.NewReader("jpeg bytes"))
	require.NoError(t, err)
	b, err := s.Put(ctx, "image/jpeg", strings.NewReader("jpeg bytes"))
	require.NoError(t, err)
	assert.Equal(t, a, b)
	assert.True(t, strings.HasPrefix(a.Key, "sha256-"))
	assert.Equal(t, int64(10), a.Size)

	c, err := s.Put(ctx, "image/jpeg", strings.NewReader("other bytes"))
	require.NoError(t, err)
	assert.NotEqual(t, a.Key, c.Key)

	rc, err := s.Open(ctx, a)
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, "jpeg bytes", string(got))
	ok, err := s.Exists(ctx, a)
	require.NoError(t, err)
	assert.True(t, ok)

	// Tamper on disk: Open refuses rather than serve the wrong image.
	path := filepath.Join(dir, "blobs", a.SHA256[:2], a.SHA256)
	require.NoError(t, os.WriteFile(path, []byte("tampered!!"), 0o644))
	_, err = s.Open(ctx, a)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum")

	_, err = s.Open(ctx, assets.Ref{Key: "../../etc/passwd", SHA256: "x"})
	require.Error(t, err)
	ok, err = s.Exists(ctx, assets.Ref{Key: "../x"})
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = s.Put(ctx, "image/jpeg", strings.NewReader(""))
	require.Error(t, err)
	_, err = s.Put(ctx, "", strings.NewReader("x"))
	require.Error(t, err)
	_, err = filestore.New(filepath.Join(dir, "missing"))
	require.Error(t, err)
}
