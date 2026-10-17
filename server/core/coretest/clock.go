// Package coretest holds fakes shared by core tests: a settable clock and the
// actors most tests need.
package coretest

import (
	"sync"
	"time"

	"github.com/WokeWorks/adx/server/core"
)

// Clock is a core.Clock a test moves by hand.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

func NewClock(at time.Time) *Clock { return &Clock{now: at.UTC()} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward and returns the new time.
func (c *Clock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	return c.now
}

// Set jumps the clock to at.
func (c *Clock) Set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = at.UTC()
}

// Actors in the default workspace.
var (
	Admin    = core.Actor{UserID: 1, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	Marketer = core.Actor{UserID: 2, Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	Viewer   = core.Actor{UserID: 3, Role: core.RoleViewer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
)
