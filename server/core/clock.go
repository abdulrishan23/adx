package core

import "time"

// Clock is where core reads the time, so tests can move it. Production uses
// SystemClock; tests use coretest.Clock.
type Clock interface {
	Now() time.Time
}

// SystemClock is the real time, in UTC.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }
