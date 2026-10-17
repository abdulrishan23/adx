package plans

import (
	"context"
	"fmt"

	"github.com/WokeWorks/adx/server/core"
)

// Revision impact: what changed between two versions, and what an accepted
// version's successor means for it.

type DiffChange struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type AllocationChange struct {
	Channel     string `json:"channel"`
	Objective   string `json:"objective"`
	BeforeMinor *int64 `json:"beforeMinor"`
	AfterMinor  *int64 `json:"afterMinor"`
	Change      string `json:"change"` // added, removed, changed
}

type PlanDiff struct {
	PlanID      int64              `json:"planId"`
	From        int64              `json:"from"`
	To          int64              `json:"to"`
	Fields      []DiffChange       `json:"fields"`
	Allocations []AllocationChange `json:"allocations"`
	// Counts of requirement rows before and after, by section.
	Sections []DiffChange `json:"sections"`
	// Invalidates names the accepted version that the later one supersedes
	// in content, if the earlier side is accepted. The acceptance itself is
	// untouched; whatever was built from it has to be re-checked against
	// the new content.
	Invalidates *Invalidation `json:"invalidates"`
}

type Invalidation struct {
	AcceptedVersion int64  `json:"acceptedVersion"`
	Reason          string `json:"reason"`
}

// Diff compares two versions of one plan.
func (p *Plans) Diff(ctx context.Context, actor core.Actor, planID, from, to int64) (PlanDiff, error) {
	a, err := p.Version(ctx, actor, planID, from)
	if err != nil {
		return PlanDiff{}, err
	}
	b, err := p.Version(ctx, actor, planID, to)
	if err != nil {
		return PlanDiff{}, err
	}
	d := PlanDiff{PlanID: planID, From: from, To: to, Fields: []DiffChange{}, Allocations: []AllocationChange{}, Sections: []DiffChange{}}
	field := func(path, before, after string) {
		if before != after {
			d.Fields = append(d.Fields, DiffChange{path, before, after})
		}
	}
	field("objective", a.Document.Objective, b.Document.Objective)
	field("budgetMinor", fmt.Sprint(a.Document.BudgetMinor), fmt.Sprint(b.Document.BudgetMinor))
	field("currency", a.Document.Currency, b.Document.Currency)
	field("periodFrom", a.Document.PeriodFrom, b.Document.PeriodFrom)
	field("periodTo", a.Document.PeriodTo, b.Document.PeriodTo)
	field("notes", a.Document.Notes, b.Document.Notes)

	key := func(al Allocation) string { return al.Channel + "|" + al.Objective }
	before := map[string]Allocation{}
	for _, al := range a.Document.Allocations {
		before[key(al)] = al
	}
	seen := map[string]bool{}
	for _, al := range b.Document.Allocations {
		k := key(al)
		seen[k] = true
		if old, ok := before[k]; ok {
			if old.BudgetMinor != al.BudgetMinor || old.Geography != al.Geography || old.Audience != al.Audience {
				ob, nb := old.BudgetMinor, al.BudgetMinor
				d.Allocations = append(d.Allocations, AllocationChange{al.Channel, al.Objective, &ob, &nb, "changed"})
			}
			continue
		}
		nb := al.BudgetMinor
		d.Allocations = append(d.Allocations, AllocationChange{al.Channel, al.Objective, nil, &nb, "added"})
	}
	for _, al := range a.Document.Allocations {
		if !seen[key(al)] {
			ob := al.BudgetMinor
			d.Allocations = append(d.Allocations, AllocationChange{al.Channel, al.Objective, &ob, nil, "removed"})
		}
	}
	section := func(name string, x, y int) {
		if x != y {
			d.Sections = append(d.Sections, DiffChange{name, fmt.Sprint(x), fmt.Sprint(y)})
		}
	}
	section("assetRequirements", len(a.Document.AssetRequirements), len(b.Document.AssetRequirements))
	section("pageRequirements", len(a.Document.PageRequirements), len(b.Document.PageRequirements))
	section("measurementRequirements", len(a.Document.MeasurementRequirements), len(b.Document.MeasurementRequirements))
	section("testHypotheses", len(a.Document.TestHypotheses), len(b.Document.TestHypotheses))
	section("evidenceRefs", len(a.Document.EvidenceRefs), len(b.Document.EvidenceRefs))
	section("assumptions", len(a.Document.Assumptions), len(b.Document.Assumptions))

	if a.State == StateAccepted && to > from && (len(d.Fields)+len(d.Allocations)+len(d.Sections)) > 0 {
		d.Invalidates = &Invalidation{AcceptedVersion: from, Reason: fmt.Sprintf("version %d changes the content accepted as version %d; anything built from version %d needs re-checking against version %d", to, from, from, to)}
	}
	return d, nil
}

// AffectedPlan is a plan version that references an offer, for the offer's
// change report.
type AffectedPlan struct {
	PlanID    int64     `json:"planId"`
	PlanTitle string    `json:"planTitle"`
	Version   int64     `json:"version"`
	State     PlanState `json:"state"`
}

// PlansUsingOffer lists plan versions that cite an offer, latest or
// accepted ones only: those are the ones a changed offer can affect.
func (p *Plans) PlansUsingOffer(ctx context.Context, actor core.Actor, offerID int64) ([]AffectedPlan, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	list, err := p.List(ctx, actor)
	if err != nil {
		return nil, err
	}
	out := []AffectedPlan{}
	for _, s := range list {
		d, err := p.Get(ctx, actor, s.ID)
		if err != nil {
			return nil, err
		}
		for i, v := range d.Versions {
			latest := i == len(d.Versions)-1
			if !latest && v.State != StateAccepted {
				continue
			}
			if usesOffer(v.Document, offerID) {
				out = append(out, AffectedPlan{PlanID: s.ID, PlanTitle: s.Title, Version: v.Version, State: v.State})
			}
		}
	}
	return out, nil
}

func usesOffer(d PlanDocument, offerID int64) bool {
	for _, r := range d.EvidenceRefs {
		if r.Kind == "offer" && r.ID == offerID {
			return true
		}
	}
	for _, r := range d.AssetRequirements {
		if r.OfferID != nil && *r.OfferID == offerID {
			return true
		}
	}
	return false
}
