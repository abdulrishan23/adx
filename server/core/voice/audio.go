package voice

import (
	"context"
	"math/rand"
	"sync"
)

// MulawCodec is the one codec ADX speaks end to end: 8 kHz mono G.711 µ-law,
// 8 bytes per millisecond, so 160 bytes is a 20 ms frame.
const (
	MulawCodec      = "mulaw_8000"
	MulawBytesPerMs = 8
	// FrameBytes is the largest frame sent to the far side (20 ms), so a
	// barge-in clears at most one frame's worth of audio that has started.
	FrameBytes = 20 * MulawBytesPerMs
)

const (
	mulawBias = 0x84
	mulawClip = 32635
)

// DecodeMulaw turns one µ-law byte into a 16-bit PCM sample (ITU-T G.711).
func DecodeMulaw(u byte) int16 {
	u = ^u
	sign := u & 0x80
	exponent := (u >> 4) & 0x07
	mantissa := u & 0x0f
	sample := (int(mantissa)<<3 + mulawBias) << exponent
	sample -= mulawBias
	if sign != 0 {
		return int16(-sample)
	}
	return int16(sample)
}

// EncodeMulaw turns a 16-bit PCM sample into one µ-law byte.
func EncodeMulaw(sample int16) byte {
	s := int(sample)
	sign := byte(0)
	if s < 0 {
		s = -s
		sign = 0x80
	}
	if s > mulawClip {
		s = mulawClip
	}
	s += mulawBias
	exponent := 7
	for mask := 0x4000; exponent > 0 && s&mask == 0; mask >>= 1 {
		exponent--
	}
	mantissa := (s >> (exponent + 3)) & 0x0f
	return ^(sign | byte(exponent<<4) | byte(mantissa))
}

// SplitFrames cuts µ-law audio into frames of at most FrameBytes, in order.
// Empty input yields no frames.
func SplitFrames(data []byte, frameBytes int) [][]byte {
	if frameBytes <= 0 {
		frameBytes = FrameBytes
	}
	var out [][]byte
	for len(data) > 0 {
		n := frameBytes
		if n > len(data) {
			n = len(data)
		}
		out = append(out, data[:n])
		data = data[n:]
	}
	return out
}

// AmbienceLoop is a soft, deterministic noise floor in µ-law: the reference
// source mixed a licensed room-tone track under the agent so silence does
// not sound like a dropped line. ADX ships no licensed audio; this is
// generated, seeded, and quiet.
func AmbienceLoop(seconds int, seed int64) []byte {
	if seconds <= 0 {
		seconds = 2
	}
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // not security-relevant: background noise shape
	out := make([]byte, seconds*8000)
	var last float64
	for i := range out {
		// Low-passed noise: mostly rumble, little hiss.
		last = 0.92*last + 0.08*(r.Float64()*2-1)
		out[i] = EncodeMulaw(int16(last * 900))
	}
	return out
}

// Ambience wraps a Transport: agent audio is mixed over a looping floor at
// Gain (0 disables mixing) and sent in frames of FrameBytes. Clear passes
// through and restarts the loop. Only the agent's leg is mixed; what the
// caller says is never touched.
type Ambience struct {
	Transport
	Loop []byte
	Gain float64
	mu   sync.Mutex
	pos  int
}

func WithAmbience(t Transport, loop []byte, gain float64) *Ambience {
	return &Ambience{Transport: t, Loop: loop, Gain: gain}
}

func (a *Ambience) Send(ctx context.Context, f Frame) error {
	data := f.Data
	if a.Gain > 0 && len(a.Loop) > 0 {
		mixed := make([]byte, len(data))
		a.mu.Lock()
		for i, b := range data {
			floor := float64(DecodeMulaw(a.Loop[a.pos])) * a.Gain
			a.pos = (a.pos + 1) % len(a.Loop)
			v := float64(DecodeMulaw(b)) + floor
			if v > 32767 {
				v = 32767
			} else if v < -32768 {
				v = -32768
			}
			mixed[i] = EncodeMulaw(int16(v))
		}
		a.mu.Unlock()
		data = mixed
	}
	for _, chunk := range SplitFrames(data, FrameBytes) {
		if err := a.Transport.Send(ctx, Frame{Codec: f.Codec, Data: chunk}); err != nil {
			return err
		}
	}
	return nil
}

func (a *Ambience) Clear(ctx context.Context) error {
	a.mu.Lock()
	a.pos = 0
	a.mu.Unlock()
	return a.Transport.Clear(ctx)
}
