package voice

import (
	"context"
	"errors"
	"sync"
	"time"
)

// BridgeConfig holds the timing the reference source arrived at.
type BridgeConfig struct {
	// StartTimeout bounds how long the speech session may take to begin.
	StartTimeout time.Duration
	// MaxDuration ends any call, however it is going.
	MaxDuration time.Duration
	// HangupGrace is how long to wait for the end-of-call mark to come back
	// before closing anyway.
	HangupGrace time.Duration
	// EndMark is the mark name sent before hangup.
	EndMark string
}

func (c BridgeConfig) withDefaults() BridgeConfig {
	if c.StartTimeout == 0 {
		c.StartTimeout = 10 * time.Second
	}
	if c.MaxDuration == 0 {
		c.MaxDuration = 10 * time.Minute
	}
	if c.HangupGrace == 0 {
		c.HangupGrace = 5 * time.Second
	}
	if c.EndMark == "" {
		c.EndMark = "end-of-call"
	}
	return c
}

// BridgeResult is what one bridged call produced.
type BridgeResult struct {
	Transcript Transcript
	// Interruptions counts barge-ins that cleared queued audio.
	Interruptions int
	// FramesDropped is audio the engine produced that a barge-in threw away.
	FramesDropped int
	// EndedBy says what ended the call: "agent", "caller", "timeout", "error".
	EndedBy string
	Err     error
}

// Bridge connects one Transport to one Session: caller audio goes to the
// engine, engine audio goes to the caller, an interruption clears what the
// caller hasn't heard yet, and the engine's end sends a mark and waits for
// it so the goodbye plays out. It owns no credentials and no database; the
// call lifecycle around it is B4.4's.
func Bridge(ctx context.Context, transport Transport, speech Speech, cfg SessionConfig, bcfg BridgeConfig) BridgeResult {
	bcfg = bcfg.withDefaults()
	ctx, cancel := context.WithTimeout(ctx, bcfg.MaxDuration)
	defer cancel()

	startCtx, cancelStart := context.WithTimeout(ctx, bcfg.StartTimeout)
	session, err := speech.Start(startCtx, cfg)
	cancelStart()
	if err != nil {
		return BridgeResult{EndedBy: "error", Err: err}
	}

	var (
		mu       sync.Mutex
		queued   []Frame // engine audio not yet sent to the transport
		result   BridgeResult
		endedBy  string
		endErr   error
		finished = make(chan struct{})
		once     sync.Once
	)
	finish := func(by string, err error) {
		once.Do(func() {
			endedBy, endErr = by, err
			close(finished)
		})
	}

	// Caller audio to the engine. A closed incoming channel means the caller
	// hung up.
	go func() {
		for {
			select {
			case <-finished:
				return
			case f, ok := <-transport.Incoming():
				if !ok {
					finish("caller", nil)
					return
				}
				if f.Codec != cfg.Codec {
					finish("error", errors.New("transport codec "+f.Codec+" does not match session codec "+cfg.Codec))
					return
				}
				if err := session.SendAudio(ctx, f); err != nil {
					finish("error", err)
					return
				}
			}
		}
	}()

	// Engine events. Audio is queued and flushed in order; an interruption
	// drops the queue and clears the far side; end marks and waits.
	go func() {
		for {
			select {
			case <-finished:
				return
			case <-ctx.Done():
				finish("timeout", nil)
				return
			case ev, ok := <-session.Events():
				if !ok {
					finish("agent", nil)
					return
				}
				switch ev.Kind {
				case KindAudio:
					mu.Lock()
					queued = append(queued, ev.Audio)
					toSend := queued
					queued = nil
					mu.Unlock()
					for _, f := range toSend {
						if err := transport.Send(ctx, f); err != nil {
							finish("error", err)
							return
						}
					}
				case KindInterruption:
					mu.Lock()
					result.Interruptions++
					result.FramesDropped += len(queued)
					queued = nil
					mu.Unlock()
					if err := transport.Clear(ctx); err != nil {
						finish("error", err)
						return
					}
				case KindTranscript:
					// Transcript lines come back from End; nothing to do live.
				case KindEnd:
					// Let everything queued play, then hang up.
					if err := transport.Mark(ctx, bcfg.EndMark); err != nil {
						finish("error", err)
						return
					}
					grace := time.NewTimer(bcfg.HangupGrace)
					defer grace.Stop()
					for {
						select {
						case name := <-transport.Marks():
							if name == bcfg.EndMark {
								finish("agent", nil)
								return
							}
						case <-grace.C:
							finish("agent", nil)
							return
						case <-ctx.Done():
							finish("timeout", nil)
							return
						}
					}
				}
			}
		}
	}()

	<-finished
	endCtx, cancelEnd := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelEnd()
	transcript, terr := session.End(endCtx)
	_ = transport.Close()
	result.Transcript = transcript
	result.EndedBy = endedBy
	result.Err = errors.Join(endErr, terr)
	return result
}
