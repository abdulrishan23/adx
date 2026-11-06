package fronts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/runs"
)

// Screens the cards open. Approvals and the supervisor's actions open the
// engine's working screens until Perform's are rewired (BD-57).
const (
	toAnalytics   = "/analytics"
	toLeads       = "/leads"
	toConnections = "/settings/connections"
	toProposals   = "/engine/proposals"
	toSupervisor  = "/engine/supervisor"
	toRuns        = "/engine/runs"
	toLaunch      = "/studio/launch"
	toPlans       = "/studio/plans"
	toResearch    = "/studio/research"
	toConcepts    = "/creative/concepts"
	toLibrary     = "/creative/library"
	toProducer    = "/creative/producer"
)

// ---------------------------------------------------------------- alerts

// alertCards makes one card per open alert that is not snoozed. Every
// alert check needs a person, so each is needs-you, with its severity as
// the status.
func (s *Service) alertCards(ctx context.Context, actor core.Actor, now time.Time) ([]Card, error) {
	if s.cfg.Alerts == nil {
		return nil, nil
	}
	var out []Card
	for _, state := range []string{"open", "acknowledged"} {
		list, err := s.cfg.Alerts.List(ctx, actor, state, 200)
		if err != nil {
			return nil, err
		}
		for _, a := range list {
			if a.Snoozed(now) {
				continue
			}
			out = append(out, alertCard(a))
		}
	}
	return out, nil
}

func alertCard(a alerts.Alert) Card {
	id := a.ID
	c := Card{ID: fmt.Sprintf("alert-%d", a.ID), Door: DoorPerform, Status: severityStatus(a.Severity), NeedsYou: true, Why: strings.TrimSpace(a.Message), At: a.FirstSeenAt, AlertID: &id}
	var ev struct {
		Day        string `json:"day"`
		SpendMinor *int64 `json:"spendMinor"`
		Currency   string `json:"currency"`
		PctAbove   *int   `json:"pctAbove"`
	}
	_ = json.Unmarshal(a.Evidence, &ev)
	switch a.Check {
	case alerts.CheckCostSpike:
		c.Title = titled("Cost spike", a.SubjectLabel)
		c.Action = &CardAction{Label: "Analytics", To: toAnalytics}
		if ev.SpendMinor != nil {
			c.Figure = &Figure{Value: str(money(*ev.SpendMinor, ev.Currency)), Count: count(*ev.SpendMinor / 100), Unit: "on " + dayLabel(reporting.Day(ev.Day))}
			if ev.PctAbove != nil {
				c.Figure.Change = fmt.Sprintf("%d%% above its average", *ev.PctAbove)
			}
		}
	case alerts.CheckZeroLeads:
		c.Title = titled("Spend with no leads", a.SubjectLabel)
		c.Action = &CardAction{Label: "Analytics", To: toAnalytics}
		if ev.SpendMinor != nil {
			c.Figure = &Figure{Value: str(money(*ev.SpendMinor, ev.Currency)), Count: count(*ev.SpendMinor / 100), Unit: "on " + dayLabel(reporting.Day(ev.Day)), Change: "no lead reached GoHighLevel"}
		}
	case alerts.CheckStaleIngestion:
		c.Title = titled("Figures stale", a.SubjectLabel)
		c.Action = &CardAction{Label: "Connections", To: toConnections}
	case "lead_hot":
		c.Door = DoorConvert
		c.Title = "A lead turned hot"
		c.Action = &CardAction{Label: "Leads", To: toLeads}
	default:
		c.Title = titled(strings.ToUpper(a.Check[:1])+strings.ReplaceAll(a.Check[1:], "_", " "), a.SubjectLabel)
	}
	if a.Occurrences > 1 {
		c.Why = fmt.Sprintf("%s Seen %d times.", c.Why, a.Occurrences)
	}
	c.Question = "Tell me more about this: " + c.Title
	return c
}

// titled joins a check's name and its subject, or stands alone when the
// check could not name the subject.
func titled(check, label string) string {
	if strings.TrimSpace(label) == "" {
		return check
	}
	return check + ": " + label
}

func severityStatus(sev string) CardStatus {
	switch sev {
	case "urgent", "high":
		return StatusError
	case "warning", "medium":
		return StatusCaution
	default:
		return StatusPending
	}
}

// ---------------------------------------------------------------- supervisor

// supervisorCards turns everything waiting on a person in the supervisor's
// inbox into cards: approvals, executions that failed or went uncertain,
// workflow steps, failed runs, call readings, CRM conflicts and candidates,
// budget conflicts, the kill switch, a missing write policy.
func (s *Service) supervisorCards(ctx context.Context, actor core.Actor, _ time.Time) ([]Card, error) {
	if s.cfg.Supervisor == nil {
		return nil, nil
	}
	items, err := s.cfg.Supervisor.Inbox(ctx, actor)
	if err != nil {
		return nil, err
	}
	out := make([]Card, 0, len(items))
	for _, it := range items {
		c := Card{ID: fmt.Sprintf("inbox-%s-%d", it.Kind, it.Ref), Door: DoorPerform, Status: severityStatus(it.Severity), NeedsYou: true, Title: it.Title, Why: it.Detail, At: it.Since,
			Action: &CardAction{Label: "Open", To: toSupervisor}}
		switch it.Kind {
		case "proposal":
			c.Action = &CardAction{Label: "Review", To: toProposals, Primary: true}
		case "call_reading", "call", "field_conflict", "match_candidate", "workflow_task":
			c.Door = DoorConvert
		case "run":
			// "Run <kind> <state>": the door follows the kind.
			if parts := strings.Fields(it.Title); len(parts) >= 2 {
				c.Door = doorForRun(parts[1])
			}
			c.Action = &CardAction{Label: "Runs", To: toRuns}
		}
		if c.Why == "" {
			c.Why = "Waiting for a person."
		}
		c.Question = "What should I do about this: " + it.Title
		out = append(out, c)
	}
	return out, nil
}

// ---------------------------------------------------------------- figures

// reportingCards are the figures worth a look: spend this month, leads per
// day and the split between platforms. They are updates, never needs-you,
// and say "Not available" rather than 0 when nothing has been read.
func (s *Service) reportingCards(ctx context.Context, actor core.Actor, now time.Time) ([]Card, error) {
	if s.cfg.Reporting == nil {
		return nil, nil
	}
	local := now.In(reporting.ReportingZone)
	today := reporting.DayOf(local)
	monthStart := reporting.DayOf(time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, reporting.ReportingZone))
	byDay, err := s.cfg.Reporting.Analytics(ctx, actor, reporting.AnalyticsQuery{From: reporting.DayOf(local.AddDate(0, 0, -29)), To: today, GroupBy: "day"})
	if err != nil {
		return nil, err
	}
	days := map[reporting.Day]reporting.ReportMetrics{}
	for _, r := range byDay.Rows {
		days[reporting.Day(strings.TrimPrefix(r.Key, "day:"))] = r.Metrics
	}
	read := byDay.Freshness.LastSuccessAt != nil
	at := now
	if read {
		at = *byDay.Freshness.LastSuccessAt
	}
	// The last 14 days, oldest first.
	var labels []string
	var spendBars, leadBars []int64
	var monthSpend *int64
	var monthLeads, monthQualified int64
	qualifiedKnown := false
	var last7, prev7 int64
	for i := 13; i >= 0; i-- {
		d := reporting.DayOf(local.AddDate(0, 0, -i))
		m := days[d]
		labels = append(labels, string(d[8:]))
		var spend int64
		if m.Spend.Minor != nil {
			spend = *m.Spend.Minor
		}
		spendBars = append(spendBars, spend/100)
		n := m.Leads
		if m.QualifiedLeads != nil {
			qualifiedKnown = true
			n = *m.QualifiedLeads
		}
		leadBars = append(leadBars, n)
		if i < 7 {
			last7 += n
		} else {
			prev7 += n
		}
	}
	for d, m := range days {
		if d < monthStart {
			continue
		}
		if m.Spend.Minor != nil {
			v := *m.Spend.Minor
			if monthSpend == nil {
				monthSpend = &v
			} else {
				*monthSpend += v
			}
		}
		monthLeads += m.Leads
		if m.QualifiedLeads != nil {
			monthQualified += *m.QualifiedLeads
		}
	}
	currency := byDay.Summary.Metrics.Spend.Currency
	var out []Card

	spend := Card{ID: "spend", Door: DoorPerform, Status: StatusReady, Title: "Spend this month", At: at, Action: &CardAction{Label: "Analytics", To: toAnalytics},
		Question: "Are we on pace this month, and which project is pulling spend?", Figure: &Figure{Unit: "since " + dayLabel(monthStart)}}
	switch {
	case !read:
		spend.Status = StatusPending
		spend.Why = "No ad account has been read yet, so there is no spend to show."
	case monthSpend == nil:
		spend.Status = StatusPending
		spend.Why = "Nothing read inside this month yet. " + through(byDay.Freshness)
	default:
		spend.Figure.Value = str(money(*monthSpend, currency))
		spend.Figure.Count = count(*monthSpend / 100)
		spend.Why = money(*monthSpend, currency) + " spent since " + dayLabel(monthStart) + ". " + through(byDay.Freshness)
		spend.Picture = &Picture{Kind: "bars", Values: spendBars, Labels: labels}
	}
	out = append(out, spend)

	if last7+prev7 > 0 || monthLeads > 0 {
		title, unit := "Leads per day", "leads this month"
		figure := monthLeads
		cost := byDay.Summary.Metrics.CostPerLead
		if qualifiedKnown {
			title, unit, figure, cost = "Qualified leads per day", "qualified this month", monthQualified, byDay.Summary.Metrics.CostPerQualifiedLead
		}
		c := Card{ID: "leads", Door: DoorPerform, Status: StatusReady, Title: title, At: at, Action: &CardAction{Label: "Leads", To: toLeads},
			Question: "Why did leads move this week, and will it hold?", Figure: &Figure{Value: str(thousands(figure)), Count: count(figure), Unit: unit},
			Picture: &Picture{Kind: "bars", Values: leadBars, Labels: labels}}
		switch {
		case cost.Value != nil:
			c.Figure.Change = money(int64(*cost.Value), currency) + " each"
		case cost.Undefined:
			c.Figure.Change = "no cost: none this period"
		default:
			c.Figure.Change = "cost not available"
		}
		yesterday := leadBars[len(leadBars)-2]
		switch {
		case prev7 == 0:
			c.Why = fmt.Sprintf("%d in the last seven days, none in the seven before. Yesterday: %d.", last7, yesterday)
		case last7 >= prev7:
			c.Why = fmt.Sprintf("Up %d%% on the week before. Yesterday: %d.", (last7-prev7)*100/prev7, yesterday)
		default:
			c.Status = StatusCaution
			c.Why = fmt.Sprintf("Down %d%% on the week before. Yesterday: %d.", (prev7-last7)*100/prev7, yesterday)
		}
		out = append(out, c)
	}

	if read {
		byPlatform, err := s.cfg.Reporting.Analytics(ctx, actor, reporting.AnalyticsQuery{From: monthStart, To: today, GroupBy: "platform"})
		if err != nil {
			return nil, err
		}
		var slices []Slice
		var total, top int64
		topName := ""
		for _, r := range byPlatform.Rows {
			if r.Metrics.Spend.Minor == nil || *r.Metrics.Spend.Minor <= 0 {
				continue
			}
			v := *r.Metrics.Spend.Minor
			slices = append(slices, Slice{Label: platformName(r.Platform), Value: v})
			total += v
			if v > top {
				top, topName = v, platformName(r.Platform)
			}
		}
		if len(slices) > 0 {
			out = append(out, Card{ID: "channels", Door: DoorPerform, Status: StatusReady, Title: "Spend by platform", At: at,
				Why: fmt.Sprintf("%s brings %d%% of spend this month.", topName, top*100/total), Picture: &Picture{Kind: "pie", Slices: slices},
				Question: "Is the split between platforms right for this month?", Action: &CardAction{Label: "Analytics", To: toAnalytics}})
		}
	}
	return out, nil
}

func through(f reporting.ResultFreshness) string {
	if f.ObservedThrough == nil {
		return "Figures read, no complete day yet."
	}
	if f.Partial {
		return "Figures through " + dayLabel(*f.ObservedThrough) + ", with gaps."
	}
	return "Figures through " + dayLabel(*f.ObservedThrough) + "."
}

func has(n int64) string {
	if n == 1 {
		return "has"
	}
	return "have"
}

func platformName(p string) string {
	switch p {
	case "google_ads":
		return "Google"
	case "meta":
		return "Meta"
	default:
		return p
	}
}

// ---------------------------------------------------------------- leads

// leadCards says how many leads have no status in GoHighLevel: the figures
// cannot count them as qualified or lost until sales sets one.
func (s *Service) leadCards(ctx context.Context, actor core.Actor, now time.Time) ([]Card, error) {
	if s.cfg.Leads == nil {
		return nil, nil
	}
	cov, err := s.cfg.Leads.Coverage(ctx, actor, 0)
	if err != nil {
		return nil, err
	}
	missing := cov.Total - cov.WithStatus
	if cov.Total == 0 || missing <= 0 {
		return nil, nil
	}
	return []Card{{ID: "leads-no-status", Door: DoorConvert, Status: StatusCaution, Title: "Leads without a status", At: now,
		Why:      fmt.Sprintf("%s of %s %s no status in GoHighLevel, so they count as neither qualified nor lost.", plural(int(missing), "lead", "leads"), thousands(cov.Total), has(missing)),
		Figure:   &Figure{Value: str(thousands(missing)), Count: count(missing), Unit: "leads"},
		Question: "Which leads have no status in GoHighLevel, and who owns them?", Action: &CardAction{Label: "Leads", To: toLeads}}}, nil
}

// ---------------------------------------------------------------- plans and launch

// planCards follows each plan from ready to delivering: a ready version
// waits for acceptance, an accepted one waits for its launch, a launch
// under way or failed says so, and a confirmed one waits for delivery.
func (s *Service) planCards(ctx context.Context, actor core.Actor, now time.Time) ([]Card, error) {
	if s.cfg.Plans == nil {
		return nil, nil
	}
	list, err := s.cfg.Plans.List(ctx, actor)
	if err != nil {
		return nil, err
	}
	today := string(reporting.DayOf(now.In(reporting.ReportingZone)))
	var out []Card
	for _, p := range list {
		d, err := s.cfg.Plans.Get(ctx, actor, p.ID)
		if err != nil {
			return nil, err
		}
		var latest, accepted *plans.PlanVersion
		for i := range d.Versions {
			v := &d.Versions[i]
			latest = v
			if v.Acceptance != nil || v.State == plans.StateAccepted {
				accepted = v
			}
		}
		if latest == nil {
			continue
		}
		if latest.Document.PeriodTo != "" && latest.Document.PeriodTo < today {
			continue
		}
		to := fmt.Sprintf("%s/%d", toPlans, p.ID)
		if latest.State == plans.StateReady && latest.Acceptance == nil {
			doc := latest.Document
			out = append(out, Card{ID: fmt.Sprintf("plan-%d-accept", p.ID), Door: DoorStudio, Status: StatusCaution, NeedsYou: true,
				Title:   fmt.Sprintf("Plan v%d for %s is waiting on you", latest.Version, p.Title),
				Why:     fmt.Sprintf("%s from %s to %s, %s. Accepting it starts the launch.", moneyShort(doc.BudgetMinor, doc.Currency), dayLabel(reporting.Day(doc.PeriodFrom)), dayLabel(reporting.Day(doc.PeriodTo)), plural(len(doc.Assumptions), "assumption", "assumptions")),
				Picture: allocationPie(doc), Question: fmt.Sprintf("Walk me through plan v%d for %s and what I'm being asked to assume.", latest.Version, p.Title),
				Action: &CardAction{Label: "Review", To: to, Primary: true}, At: latest.CreatedAt})
		}
		if accepted == nil || s.cfg.Execution == nil {
			continue
		}
		ls, err := s.cfg.Execution.Launch(ctx, actor, p.ID, accepted.Version)
		if err != nil {
			return nil, err
		}
		// An imported version can be accepted without an acceptance record;
		// its creation is then the best date there is.
		acceptedAt := accepted.CreatedAt
		if accepted.Acceptance != nil {
			acceptedAt = accepted.Acceptance.AcceptedAt
		}
		base := Card{ID: fmt.Sprintf("plan-%d-launch", p.ID), Door: DoorStudio, At: acceptedAt, Action: &CardAction{Label: "Launch", To: toLaunch}}
		switch {
		case ls.Delivered:
			continue
		case ls.State == "none":
			base.Status, base.NeedsYou = StatusCaution, true
			base.Title = fmt.Sprintf("%s is accepted, not launched", p.Title)
			base.Why = fmt.Sprintf("Plan v%d was accepted %s ago and nothing has been sent to a platform.", accepted.Version, waited(now.Sub(acceptedAt)))
			base.Question = fmt.Sprintf("What's blocking the %s launch?", p.Title)
			base.Action.Primary = true
		case ls.State == "failed":
			base.Status, base.NeedsYou = StatusError, true
			base.Title = fmt.Sprintf("The %s launch failed", p.Title)
			base.Why = "Every intent sent to a platform failed or was cancelled. " + platformNote(ls)
			base.Question = fmt.Sprintf("Why did the %s launch fail, and what do I do now?", p.Title)
		case ls.State == "partial":
			base.Status, base.NeedsYou = StatusCaution, true
			base.Title = fmt.Sprintf("The %s launch is partly blocked", p.Title)
			base.Why = "Some intents failed while others went through. " + platformNote(ls)
			base.Question = fmt.Sprintf("What's blocking the %s launch?", p.Title)
		case ls.State == "confirmed":
			base.Status = StatusPending
			base.Title = fmt.Sprintf("%s launched, waiting for delivery", p.Title)
			base.Why = "Every platform confirmed its campaigns exist. Delivery is read from their figures, not inferred."
			base.Question = fmt.Sprintf("Is %s delivering yet?", p.Title)
		default: // in_progress, submitted
			base.Status = StatusPending
			base.Title = fmt.Sprintf("Launching %s", p.Title)
			base.Why = "Intents are with the platforms. " + platformNote(ls)
			base.Question = fmt.Sprintf("Where is the %s launch?", p.Title)
		}
		out = append(out, base)
	}
	return out, nil
}

// platformNote reads "Google: submitted. Meta: failed." off the status.
func platformNote(ls execution.LaunchStatus) string {
	var parts []string
	for _, p := range ls.Platforms {
		parts = append(parts, platformName(p.Platform)+": "+strings.ReplaceAll(p.State, "_", " ")+".")
	}
	return strings.Join(parts, " ")
}

func allocationPie(doc plans.PlanDocument) *Picture {
	byChannel := map[string]int64{}
	var order []string
	for _, a := range doc.Allocations {
		label := a.Channel
		if ch, ok := plans.Channel(a.Channel); ok {
			label = ch.Label
		}
		if _, seen := byChannel[label]; !seen {
			order = append(order, label)
		}
		byChannel[label] += a.BudgetMinor
	}
	if len(order) == 0 {
		return nil
	}
	ev := &Picture{Kind: "pie"}
	for _, l := range order {
		ev.Slices = append(ev.Slices, Slice{Label: l, Value: byChannel[l]})
	}
	return ev
}

// ---------------------------------------------------------------- creative review

// reviewCards says, per project, how many ads wait for a decision on the
// review board.
func (s *Service) reviewCards(ctx context.Context, actor core.Actor, _ time.Time) ([]Card, error) {
	if s.cfg.Studio == nil {
		return nil, nil
	}
	ov, err := s.cfg.Studio.Overview(ctx, actor)
	if err != nil {
		return nil, err
	}
	var out []Card
	for _, p := range ov.Projects {
		if p.Waiting == 0 {
			continue
		}
		out = append(out, Card{ID: fmt.Sprintf("review-%d", p.ProjectID), Door: DoorStudio, Status: StatusCaution, NeedsYou: true,
			Title: fmt.Sprintf("%s waiting on review", plural(p.Waiting, "creative", "creatives")), Why: fmt.Sprintf("%s has %s with no decision on the review board.", p.Name, plural(p.Waiting, "ad", "ads")),
			Figure: &Figure{Value: str(thousands(int64(p.Waiting))), Count: count(int64(p.Waiting)), Unit: "waiting"}, Question: fmt.Sprintf("Which creatives for %s are waiting on review?", p.Name),
			Action: &CardAction{Label: "Review", To: toConcepts}, At: p.UpdatedAt})
	}
	return out, nil
}

// ---------------------------------------------------------------- runs

// finishedRunCards are the results of jobs that finished in the last day.
// Failed runs come through the supervisor's inbox instead.
func (s *Service) finishedRunCards(ctx context.Context, actor core.Actor, now time.Time) ([]Card, error) {
	if s.cfg.Runs == nil {
		return nil, nil
	}
	list, err := s.cfg.Runs.List(ctx, actor, 200)
	if err != nil {
		return nil, err
	}
	var out []Card
	for _, r := range list {
		if r.State != runs.StateSucceeded && r.State != runs.StatePartial || r.FinishedAt == nil || now.Sub(*r.FinishedAt) > 24*time.Hour {
			continue
		}
		title, to := runDone(r.Kind)
		c := Card{ID: "run-" + r.ID, Door: doorForRun(r.Kind), Status: StatusReady, Title: title, At: *r.FinishedAt, Question: "What did this job find: " + title + "?",
			Why: "Finished " + waited(now.Sub(*r.FinishedAt)) + " ago.", Action: &CardAction{Label: "Open", To: to}}
		if r.State == runs.StatePartial || r.Completeness == runs.CompletenessPartial {
			c.Status = StatusCaution
			c.Why += " Some of it is missing."
		}
		out = append(out, c)
	}
	return out, nil
}

// working lists the jobs queued or running now.
func (s *Service) working(ctx context.Context, actor core.Actor, now time.Time) ([]WorkItem, error) {
	if s.cfg.Runs == nil {
		return nil, nil
	}
	list, err := s.cfg.Runs.List(ctx, actor, 200)
	if err != nil {
		return nil, err
	}
	var out []WorkItem
	for _, r := range list {
		if r.State != runs.StateQueued && r.State != runs.StateRunning {
			continue
		}
		detail := "Queued."
		if r.StartedAt != nil {
			detail = "Started " + waited(now.Sub(*r.StartedAt)) + " ago."
		}
		out = append(out, WorkItem{ID: r.ID, Title: runDoing(r.Kind), Detail: detail, To: toRuns})
	}
	return out, nil
}

func doorForRun(kind string) Door {
	switch {
	case strings.HasPrefix(kind, "ads."):
		return DoorPerform
	case strings.HasPrefix(kind, "ghl."):
		return DoorConvert
	default:
		return DoorStudio
	}
}

func runDoing(kind string) string {
	switch kind {
	case "ads.refresh":
		return "Refreshing the figures"
	case "ghl.import":
		return "Importing leads from GoHighLevel"
	case "research.run":
		return "Running research"
	case "creative.render":
		return "Rendering a creative"
	case "plan_generation":
		return "Drafting a plan"
	case "studio_video":
		return "Making a video"
	default:
		return "Running " + kind
	}
}

func runDone(kind string) (string, string) {
	switch kind {
	case "ads.refresh":
		return "Figures refreshed", toAnalytics
	case "ghl.import":
		return "Leads imported", toLeads
	case "research.run":
		return "Research refreshed", toResearch
	case "creative.render":
		return "A creative rendered", toLibrary
	case "plan_generation":
		return "A plan drafted", toPlans
	case "studio_video":
		return "A video made", toProducer
	default:
		return kind + " finished", toRuns
	}
}
