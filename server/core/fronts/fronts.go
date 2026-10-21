// Package fronts composes the cards Home, the Inbox and Present show: what
// deserves attention today, from any door, each with a status, one line on
// why it is there, a figure or a picture, a question for "Talk about this"
// and at most one action. Nothing here is a new fact: every card is read
// from a domain that already keeps it (alerts, the supervisor's inbox, the
// figures, plans and their launch, runs, the creative review board), and
// every figure is computed in code. A missing figure is null, never 0.
//
// The order is the Inbox's: what needs a person first, worst first, then
// updates, newest first. Home's carousel and Present show the same order.
package fronts

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/core/studio"
	"github.com/WokeWorks/adx/server/core/supervisor"
)

// Door is the sub-product a card belongs to. Templates may belong to any.
type Door string

const (
	DoorStudio  Door = "studio"
	DoorPerform Door = "perform"
	DoorConvert Door = "convert"
	DoorAny     Door = "any"
)

// Status is the card's mark: the only colour on the screen.
type CardStatus string

const (
	StatusReady   CardStatus = "ready"
	StatusCaution CardStatus = "caution"
	StatusError   CardStatus = "error"
	StatusPending CardStatus = "pending"
)

// Action is where the card's one button goes: a plain path, so a front
// never has to know a screen's search params.
type CardAction struct {
	Label   string `json:"label"`
	To      string `json:"to"`
	Primary bool   `json:"primary"`
}

// Figure is the card's big number. Value is null when the figure is not
// available, and the screen says so; it is never 0 for a missing figure.
// Count is the number behind the text, for the count-up, when there is one.
type Figure struct {
	Value  *string  `json:"value"`
	Count  *float64 `json:"count"`
	Unit   string   `json:"unit"`
	Change string   `json:"change"`
}

// BarRow is one labelled bar: the text of its value and its share of the
// largest, 0 to 100.
type BarRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Share int    `json:"share"`
}

type Slice struct {
	Label string `json:"label"`
	Value int64  `json:"value"`
}

type LeadItem struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
	Waited string `json:"waited"`
}

type AdItem struct {
	Code   string     `json:"code"`
	Status CardStatus `json:"status"`
	Note   string     `json:"note"`
}

// Picture is the picture a card carries. Kind says which fields apply:
// bars (Values, Labels), pie (Slices), rows (Rows), pace (Rows, Trend),
// leads (Leads), ads (ProjectID, Ads).
type Picture struct {
	Kind      string     `json:"kind"`
	Values    []int64    `json:"values,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	Slices    []Slice    `json:"slices,omitempty"`
	Rows      []BarRow   `json:"rows,omitempty"`
	Trend     []int64    `json:"trend,omitempty"`
	Leads     []LeadItem `json:"leads,omitempty"`
	ProjectID int64      `json:"projectId,omitempty"`
	Ads       []AdItem   `json:"ads,omitempty"`
}

// Card is one thing that deserves attention today.
type Card struct {
	ID     string     `json:"id"`
	Door   Door       `json:"door"`
	Status CardStatus `json:"status"`
	// NeedsYou says a person has to act: an approval, a block, an alert, a
	// review. The rest are updates: a figure worth a look, a job finished.
	NeedsYou bool   `json:"needsYou"`
	Title    string `json:"title"`
	// Why is one line on why the card is here today.
	Why     string   `json:"why"`
	Figure  *Figure  `json:"figure"`
	Picture *Picture `json:"picture"`
	// Question is what "Talk about this" asks.
	Question string      `json:"question"`
	Action   *CardAction `json:"action"`
	// At is when the card arrived, so the Inbox can list newest first.
	At time.Time `json:"at"`
	// AlertID is set when the card is an alert, so the Inbox can snooze or
	// dismiss it.
	AlertID *int64 `json:"alertId"`
}

// WorkItem is a job under way. Nobody needs to watch it: the Inbox shows a
// count, and the result arrives as a card.
type WorkItem struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	To     string `json:"to"`
}

// Desk is Home: the summary line, every card in order, the jobs under way
// and the prompt templates.
type Desk struct {
	CheckedAt time.Time  `json:"checkedAt"`
	Summary   string     `json:"summary"`
	Cards     []Card     `json:"cards"`
	Working   []WorkItem `json:"working"`
	Templates []Template `json:"templates"`
}

// Counts are how many cards each Inbox filter would show.
type Counts struct {
	All      int `json:"all"`
	NeedsYou int `json:"needsYou"`
	Updates  int `json:"updates"`
}

// Inbox is the Inbox screen: the cards the filter keeps, in order, with the
// counts of every filter so the segments can say what they hold.
type Inbox struct {
	Summary string     `json:"summary"`
	Counts  Counts     `json:"counts"`
	Cards   []Card     `json:"cards"`
	Working []WorkItem `json:"working"`
}

// InboxFilter narrows the Inbox. Show is "", all, needs or updates; Door is
// "" or any for every door.
type InboxFilter struct {
	Show string
	Door Door
}

// Config names the domains the fronts read. A nil one is left out, which
// tests use to compose from one source at a time.
type Config struct {
	Clock      core.Clock
	Alerts     *alerts.Alerts
	Supervisor *supervisor.Supervisor
	Reporting  *reporting.Reporting
	Runs       *runs.Runs
	Plans      *plans.Plans
	Execution  *execution.Dispatcher
	Studio     *studio.Studio
	Leads      *leads.Leads
}

type Service struct {
	cfg   Config
	clock core.Clock
}

func New(cfg Config) *Service {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Service{cfg: cfg, clock: cfg.Clock}
}

// Desk composes Home.
func (s *Service) Desk(ctx context.Context, actor core.Actor) (Desk, error) {
	cards, working, err := s.compose(ctx, actor)
	if err != nil {
		return Desk{}, err
	}
	return Desk{CheckedAt: s.clock.Now().UTC(), Summary: Summary(cards), Cards: cards, Working: working, Templates: Templates()}, nil
}

// Inbox composes the Inbox under a filter. The counts are of every card,
// before the filter.
func (s *Service) Inbox(ctx context.Context, actor core.Actor, f InboxFilter) (Inbox, error) {
	switch f.Show {
	case "", "all", "needs", "updates":
	default:
		return Inbox{}, core.Invalid("show must be all, needs or updates")
	}
	switch f.Door {
	case "", DoorAny, DoorStudio, DoorPerform, DoorConvert:
	default:
		return Inbox{}, core.Invalid("door must be studio, perform, convert or any")
	}
	cards, working, err := s.compose(ctx, actor)
	if err != nil {
		return Inbox{}, err
	}
	in := Inbox{Summary: Summary(cards), Cards: []Card{}, Working: working}
	for _, c := range cards {
		in.Counts.All++
		if c.NeedsYou {
			in.Counts.NeedsYou++
		} else {
			in.Counts.Updates++
		}
		if f.Show == "needs" && !c.NeedsYou || f.Show == "updates" && c.NeedsYou {
			continue
		}
		if f.Door != "" && f.Door != DoorAny && c.Door != f.Door {
			continue
		}
		in.Cards = append(in.Cards, c)
	}
	return in, nil
}

// compose gathers every card from every configured domain and puts them in
// the Inbox's order.
func (s *Service) compose(ctx context.Context, actor core.Actor) ([]Card, []WorkItem, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, nil, core.ErrForbidden
	}
	now := s.clock.Now().UTC()
	cards := []Card{}
	working := []WorkItem{}
	for _, source := range []func(context.Context, core.Actor, time.Time) ([]Card, error){
		s.alertCards, s.supervisorCards, s.reportingCards, s.leadCards, s.planCards, s.reviewCards, s.finishedRunCards,
	} {
		more, err := source(ctx, actor, now)
		if err != nil {
			return nil, nil, err
		}
		cards = append(cards, more...)
	}
	w, err := s.working(ctx, actor, now)
	if err != nil {
		return nil, nil, err
	}
	working = append(working, w...)
	Order(cards)
	return cards, working, nil
}

var rank = map[CardStatus]int{StatusError: 0, StatusCaution: 1, StatusPending: 2, StatusReady: 3}

// Order sorts cards the Inbox's way: what needs a person first, worst
// first, then updates; newest first within the same rank.
func Order(cards []Card) {
	sort.SliceStable(cards, func(i, j int) bool {
		a, b := cards[i], cards[j]
		if a.NeedsYou != b.NeedsYou {
			return a.NeedsYou
		}
		if rank[a.Status] != rank[b.Status] {
			return rank[a.Status] < rank[b.Status]
		}
		return a.At.After(b.At)
	})
}

// Summary is the line under the greeting.
func Summary(cards []Card) string {
	n := 0
	for _, c := range cards {
		if c.NeedsYou {
			n++
		}
	}
	switch n {
	case 0:
		return "Nothing needs your attention."
	case 1:
		return "1 thing needs your attention."
	default:
		return fmt.Sprintf("%d things need your attention.", n)
	}
}

func str(s string) *string { return &s }

func count(n int64) *float64 {
	f := float64(n)
	return &f
}

// money writes minor units as "AED 1,234". No decimals: the cards are a
// glance, and the screen that opens has the exact figure.
func money(minor int64, currency string) string {
	if currency == "" {
		currency = "AED"
	}
	return currency + " " + thousands(minor/100)
}

// moneyShort writes "AED 1.2M", "AED 45.3K" or "AED 950".
func moneyShort(minor int64, currency string) string {
	if currency == "" {
		currency = "AED"
	}
	major := float64(minor) / 100
	switch {
	case major >= 1_000_000:
		return fmt.Sprintf("%s %sM", currency, trim(major/1_000_000))
	case major >= 1_000:
		return fmt.Sprintf("%s %sK", currency, trim(major/1_000))
	default:
		return fmt.Sprintf("%s %d", currency, int64(major))
	}
}

func trim(f float64) string {
	s := fmt.Sprintf("%.1f", f)
	if len(s) > 2 && s[len(s)-2:] == ".0" {
		return s[:len(s)-2]
	}
	return s
}

func thousands(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// waited says how long something has sat, in the unit that reads naturally.
func waited(d time.Duration) string {
	hours := int(d.Round(time.Hour).Hours())
	switch {
	case hours < 1:
		return "under an hour"
	case hours < 48:
		return fmt.Sprintf("%d h", hours)
	default:
		return fmt.Sprintf("%d days", hours/24)
	}
}

func dayLabel(d reporting.Day) string {
	t, err := d.Time()
	if err != nil {
		return string(d)
	}
	return t.Format("2 Jan")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
