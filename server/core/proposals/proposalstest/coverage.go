// Package proposalstest holds fakes for tests that propose changes: plan
// coverage that says every campaign is under a plan, or none is.
package proposalstest

import "context"

// Planned says every campaign has an accepted plan behind it.
type Planned struct{}

func (Planned) UnderPlan(context.Context, int64, int64, string) (bool, string, error) {
	return true, "planned (test fake)", nil
}

// Unplanned says no campaign has a plan behind it.
type Unplanned struct{}

func (Unplanned) UnderPlan(context.Context, int64, int64, string) (bool, string, error) {
	return false, "unplanned (test fake)", nil
}
