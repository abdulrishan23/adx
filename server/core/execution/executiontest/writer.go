// Package executiontest is a scripted platform writer: it "creates" objects
// in memory under their marker, can fail or time out on demand, and finds
// what it created so reconciliation can be tested without a platform.
package executiontest

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
)

type Object struct {
	ID     string
	Name   string
	Status string
	Marker string
	Action policies.WriteAction
}

type Writer struct {
	mu       sync.Mutex
	platform string
	caps     []policies.WriteAction
	// Objects created, by marker.
	Objects map[string]Object
	// UncertainNext makes the next Apply create the object and then return
	// an uncertain error, like a timeout after the request landed.
	UncertainNext bool
	// LostNext makes the next Apply return an uncertain error without
	// creating anything, like a request that never arrived.
	LostNext bool
	// FailNext makes the next Apply fail outright.
	FailNext error
	// Hidden makes Verify report objects as not visible.
	Hidden   bool
	Applies  int
	Lookups  int
	Verifies int
	next     int
}

func New(platform string, caps ...policies.WriteAction) *Writer {
	return &Writer{platform: platform, caps: caps, Objects: map[string]Object{}}
}

func (w *Writer) Platform() string                     { return w.platform }
func (w *Writer) Capabilities() []policies.WriteAction { return w.caps }

func (w *Writer) Apply(_ context.Context, req execution.ApplyRequest) (execution.ApplyResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Applies++
	if w.FailNext != nil {
		err := w.FailNext
		w.FailNext = nil
		return execution.ApplyResult{}, err
	}
	if w.LostNext {
		w.LostNext = false
		return execution.ApplyResult{}, &execution.UncertainError{Cause: errors.New("timeout before the request was sent")}
	}
	w.next++
	id := req.Marker + "-" + string(rune('a'+w.next-1))
	obj := Object{ID: id, Name: "[" + req.Marker + "]", Status: "PAUSED", Marker: req.Marker, Action: req.Action}
	if req.Action != policies.CampaignCreatePaus {
		obj.ID = req.TargetRef
		obj.Status = "UPDATED"
	}
	w.Objects[req.Marker] = obj
	if w.UncertainNext {
		w.UncertainNext = false
		return execution.ApplyResult{}, &execution.UncertainError{Cause: errors.New("timeout after the request was sent")}
	}
	return execution.ApplyResult{Refs: []proposals.ProviderRef{{Platform: w.platform, Kind: "campaign", ExternalID: obj.ID, At: time.Now()}}, Detail: "fake " + string(req.Action)}, nil
}

func (w *Writer) Lookup(_ context.Context, _, _ string, marker string, _ policies.WriteAction, _ string) (execution.LookupResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Lookups++
	obj, ok := w.Objects[marker]
	if !ok {
		return execution.LookupResult{Found: false, Detail: "no object carries marker " + marker}, nil
	}
	return execution.LookupResult{Found: true, Status: obj.Status, Detail: "found " + obj.ID, Refs: []proposals.ProviderRef{{Platform: w.platform, Kind: "campaign", ExternalID: obj.ID, At: time.Now()}}}, nil
}

func (w *Writer) Verify(_ context.Context, _, _ string, refs []proposals.ProviderRef) (execution.VerifyResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Verifies++
	if w.Hidden {
		return execution.VerifyResult{Exists: false, Detail: "not visible"}, nil
	}
	for _, o := range w.Objects {
		for _, r := range refs {
			if o.ID == r.ExternalID {
				return execution.VerifyResult{Exists: true, Status: o.Status, Detail: "exists"}, nil
			}
		}
	}
	return execution.VerifyResult{Exists: false, Detail: "unknown ref"}, nil
}

// Count is how many objects were created for a marker prefix.
func (w *Writer) Count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.Objects)
}
