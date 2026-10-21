package ffmpeg

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pic(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 540, 960))
	for y := 0; y < 960; y++ {
		for x := 0; x < 540; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

// Catches: a build that produces no playable file, a poster missing, and
// the card not extending the length.
func TestBuildMotionAndPoster(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	b, err := New("")
	require.NoError(t, err)
	mp4, poster, err := b.Build(t.Context(), Motion{
		Pictures: [][]byte{pic(t, color.RGBA{30, 60, 120, 255}), pic(t, color.RGBA{120, 60, 30, 255})},
		Card:     pic(t, color.RGBA{10, 10, 10, 255}),
		Width:    360, Height: 640, Seconds: 1.5, CardHold: 1, FrameRate: 15,
	})
	require.NoError(t, err)
	assert.True(t, bytes.Contains(mp4[:64], []byte("ftyp")), "an MP4 container")
	assert.Greater(t, len(mp4), 2000)
	_, err = jpeg.Decode(bytes.NewReader(poster))
	require.NoError(t, err, "the poster is a JPEG")

	dir := t.TempDir()
	path := dir + "/out.mp4"
	require.NoError(t, os.WriteFile(path, mp4, 0o600))
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	require.NoError(t, err)
	var seconds float64
	_, err = fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &seconds)
	require.NoError(t, err)
	// Two pictures of 1.5 s and a card of 1 s, with two half-second dissolves.
	assert.InDelta(t, 3.0, seconds, 0.3)
}
