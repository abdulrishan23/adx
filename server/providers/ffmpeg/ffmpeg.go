// Package ffmpeg builds motion video from still pictures with the ffmpeg
// binary: each picture drifts and zooms for a few seconds, pictures
// dissolve into one another, and a last card holds. It also takes a poster
// frame out of any video. Nothing here chooses pictures or words; the
// studio does, and hands over bytes.
package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Builder struct {
	bin string
}

// New finds ffmpeg on the path, or at bin when given.
func New(bin string) (*Builder, error) {
	if bin == "" {
		bin = "ffmpeg"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found: %w", err)
	}
	return &Builder{bin: path}, nil
}

// Motion is the spec of one motion video: pictures in order, an optional
// final card (the finished static ad), the size and timing.
type Motion struct {
	Pictures  [][]byte // JPEG or PNG
	Card      []byte   // held at the end; nil for none
	Width     int
	Height    int
	Seconds   float64 // per picture
	CardHold  float64 // seconds the card holds
	FrameRate int
}

// Build renders the motion as H.264 MP4 and returns it with a poster JPEG
// taken from the first second.
func (b *Builder) Build(ctx context.Context, m Motion) (mp4, poster []byte, err error) {
	if len(m.Pictures) == 0 && m.Card == nil {
		return nil, nil, errors.New("ffmpeg: nothing to build from")
	}
	if m.Width <= 0 || m.Height <= 0 {
		m.Width, m.Height = 1080, 1920
	}
	if m.Seconds <= 0 {
		m.Seconds = 3.5
	}
	if m.CardHold <= 0 {
		m.CardHold = 3
	}
	if m.FrameRate <= 0 {
		m.FrameRate = 30
	}
	dir, err := os.MkdirTemp("", "adx-motion-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(dir)

	var inputs []string
	var filters []string
	var labels []string
	n := 0
	add := func(data []byte, seconds float64, zoom bool) error {
		path := filepath.Join(dir, fmt.Sprintf("in%d.img", n))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
		inputs = append(inputs, "-loop", "1", "-t", fmt.Sprintf("%.2f", seconds), "-i", path)
		frames := int(seconds * float64(m.FrameRate))
		// Scale to cover, then crop to size; the zoom drifts in a little
		// over the picture's time. A still card just holds.
		f := fmt.Sprintf("[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1", n, m.Width*2, m.Height*2, m.Width*2, m.Height*2)
		if zoom {
			f += fmt.Sprintf(",zoompan=z='min(zoom+0.0008,1.12)':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=%d", frames, m.Width, m.Height, m.FrameRate)
		} else {
			f += fmt.Sprintf(",scale=%d:%d,fps=%d", m.Width, m.Height, m.FrameRate)
		}
		f += fmt.Sprintf(",format=yuv420p[v%d]", n)
		filters = append(filters, f)
		labels = append(labels, fmt.Sprintf("[v%d]", n))
		n++
		return nil
	}
	for _, p := range m.Pictures {
		if err := add(p, m.Seconds, true); err != nil {
			return nil, nil, err
		}
	}
	if m.Card != nil {
		if err := add(m.Card, m.CardHold, false); err != nil {
			return nil, nil, err
		}
	}
	// Dissolve each clip into the next, half a second each.
	const fade = 0.5
	last := labels[0]
	offset := 0.0
	durations := make([]float64, 0, n)
	for i := range m.Pictures {
		_ = i
		durations = append(durations, m.Seconds)
	}
	if m.Card != nil {
		durations = append(durations, m.CardHold)
	}
	for i := 1; i < n; i++ {
		offset += durations[i-1] - fade
		out := fmt.Sprintf("[x%d]", i)
		filters = append(filters, fmt.Sprintf("%s%sxfade=transition=fade:duration=%.2f:offset=%.2f%s", last, labels[i], fade, offset, out))
		last = out
	}
	outPath := filepath.Join(dir, "out.mp4")
	args := append([]string{"-y", "-hide_banner", "-loglevel", "error"}, inputs...)
	args = append(args, "-filter_complex", strings.Join(filters, ";"), "-map", last,
		"-c:v", "libx264", "-preset", "medium", "-crf", "20", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-an", outPath)
	if err := b.run(ctx, args, 10*time.Minute); err != nil {
		return nil, nil, err
	}
	mp4, err = os.ReadFile(outPath)
	if err != nil {
		return nil, nil, err
	}
	poster, err = b.Poster(ctx, mp4)
	if err != nil {
		return nil, nil, err
	}
	return mp4, poster, nil
}

// Poster takes a JPEG frame from near the start of a video.
func (b *Builder) Poster(ctx context.Context, mp4 []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "adx-poster-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.mp4")
	out := filepath.Join(dir, "poster.jpg")
	if err := os.WriteFile(in, mp4, 0o600); err != nil {
		return nil, err
	}
	if err := b.run(ctx, []string{"-y", "-hide_banner", "-loglevel", "error", "-ss", "0.5", "-i", in, "-frames:v", "1", "-q:v", "3", out}, 2*time.Minute); err != nil {
		return nil, err
	}
	return os.ReadFile(out)
}

func (b *Builder) run(ctx context.Context, args []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return fmt.Errorf("ffmpeg: %w: %s", err, msg)
	}
	return nil
}
