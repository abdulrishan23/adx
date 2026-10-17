package research

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/learning"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/reports"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
)

// Recurring research. A schedule re-reads a brief on a cadence and writes a
// comparative report for the period. The report puts side by side what was
// observed (our figures against the previous period, competitor pages
// against their last read, keyword estimates against the last run,
// recorded market events) and proposes hypotheses that cite that evidence.
// A hypothesis is always a hypothesis: nothing here infers a cause, and an
// operator's acceptance is recorded as the team's judgement, not as proof.

type Schedule struct {
	ID        int64           `json:"id"`
	BriefID   int64           `json:"briefId"`
	Name      string          `json:"name"`
	Cadence   reports.Cadence `json:"cadence"`
	Enabled   bool            `json:"enabled"`
	CreatedBy *int64          `json:"createdBy"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type ScheduleInput struct {
	BriefID int64           `json:"briefId"`
	Name    string          `json:"name"`
	Cadence reports.Cadence `json:"cadence"`
	Enabled bool            `json:"enabled"`
}

// MarketEvent is something a person recorded as having happened in the
// market: a festival period, an economic announcement, a regulation, a
// competitor launch. It is evidence (manual, observed) and the only way a
// festival or economic explanation enters a report.
type MarketEvent struct {
	EvidenceID int64     `json:"evidenceId"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	SourceURL  string    `json:"sourceUrl"`
	Note       string    `json:"note"`
	RecordedBy *int64    `json:"recordedBy"`
	RecordedAt time.Time `json:"recordedAt"`
}

type MarketEventInput struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	From      string `json:"from"`
	To        string `json:"to"`
	SourceURL string `json:"sourceUrl"`
	Note      string `json:"note"`
}

type DateRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type AccountComparison struct {
	Summary        reporting.ReportRow       `json:"summary"`
	Rows           []reporting.ReportRow     `json:"rows"`
	Freshness      reporting.ResultFreshness `json:"freshness"`
	Coverage       reporting.ReportCoverage  `json:"coverage"`
	Warnings       []reporting.ReportWarning `json:"warnings"`
	FormulaVersion string                    `json:"formulaVersion"`
	EvidenceID     int64                     `json:"evidenceId"`
}

type PageComparison struct {
	ItemID             int64   `json:"itemId"`
	URL                string  `json:"url"`
	Title              string  `json:"title"`
	Changed            bool    `json:"changed"`
	EvidenceID         int64   `json:"evidenceId"`
	PreviousEvidenceID *int64  `json:"previousEvidenceId"`
	ExcerptBefore      string  `json:"excerptBefore"`
	ExcerptAfter       string  `json:"excerptAfter"`
	ObservedAt         string  `json:"observedAt"`
	PreviousObservedAt *string `json:"previousObservedAt"`
}

type CompetitorComparison struct {
	BrandID int64            `json:"brandId"`
	Name    string           `json:"name"`
	Pages   []PageComparison `json:"pages"`
}

type KeywordComparison struct {
	Text       string   `json:"text"`
	Before     *int64   `json:"before"`
	After      *int64   `json:"after"`
	ChangePct  *float64 `json:"changePct"`
	Class      string   `json:"class"`
	EvidenceID int64    `json:"evidenceId"`
}

// Hypothesis pairs what was observed with a possible cause. Class is always
// "hypothesis" and Causal always false: the report shows a coincidence in
// time, not a mechanism.
type Hypothesis struct {
	Key          string  `json:"key"`
	Kind         string  `json:"kind"`
	Observed     string  `json:"observed"`
	Hypothetical string  `json:"hypothetical"`
	EvidenceRefs []int64 `json:"evidenceRefs"`
	Class        string  `json:"class"`
	Causal       bool    `json:"causal"`
}

type PriorHypothesis struct {
	Key    string  `json:"key"`
	Kind   string  `json:"kind"`
	Text   string  `json:"text"`
	Status *string `json:"status"`
	Note   string  `json:"note"`
}

type PriorContext struct {
	ReportID    int64             `json:"reportId"`
	PeriodStart string            `json:"periodStart"`
	PeriodEnd   string            `json:"periodEnd"`
	Hypotheses  []PriorHypothesis `json:"hypotheses"`
}

type ReportDocument struct {
	Period      DateRange              `json:"period"`
	Previous    DateRange              `json:"previous"`
	Account     *AccountComparison     `json:"account"`
	Competitors []CompetitorComparison `json:"competitors"`
	Keywords    []KeywordComparison    `json:"keywords"`
	Events      []MarketEvent          `json:"events"`
	Hypotheses  []Hypothesis           `json:"hypotheses"`
	Prior       *PriorContext          `json:"prior"`
	// Findings are organizational memory (core/learning): validated and
	// supported insights with their lineage, carried into the report.
	Findings []learning.MemoryFinding `json:"findings"`
}

type HypothesisReview struct {
	Key       string    `json:"key"`
	Status    string    `json:"status"`
	Note      string    `json:"note"`
	DecidedBy int64     `json:"decidedBy"`
	DecidedAt time.Time `json:"decidedAt"`
}

type ComparativeReport struct {
	ID               int64              `json:"id"`
	BriefID          int64              `json:"briefId"`
	ScheduleID       *int64             `json:"scheduleId"`
	OccurrenceKey    string             `json:"occurrenceKey"`
	PeriodStart      string             `json:"periodStart"`
	PeriodEnd        string             `json:"periodEnd"`
	PreviousReportID *int64             `json:"previousReportId"`
	Limited          bool               `json:"limited"`
	LimitedReason    string             `json:"limitedReason"`
	Document         ReportDocument     `json:"document"`
	Reviews          []HypothesisReview `json:"reviews"`
	GeneratedAt      time.Time          `json:"generatedAt"`
}

const dateLayout = "2006-01-02"

// Schedules.

func (r *Research) CreateSchedule(ctx context.Context, actor core.Actor, in ScheduleInput) (Schedule, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Schedule{}, core.ErrForbidden
	}
	if err := validateSchedule(in); err != nil {
		return Schedule{}, err
	}
	if _, err := r.brief(ctx, actor.WorkspaceID, in.BriefID); err != nil {
		return Schedule{}, err
	}
	now := r.clock.Now().UTC()
	row, err := r.db.Writer().CreateResearchSchedule(ctx, store.CreateResearchScheduleParams{WorkspaceID: actor.WorkspaceID, BriefID: in.BriefID, Name: strings.TrimSpace(in.Name), Cadence: string(in.Cadence), Enabled: boolInt(in.Enabled), CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return Schedule{}, err
	}
	return scheduleFromRow(row), nil
}

func (r *Research) UpdateSchedule(ctx context.Context, actor core.Actor, id int64, in ScheduleInput) (Schedule, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Schedule{}, core.ErrForbidden
	}
	cur, err := r.db.Reader().GetResearchSchedule(ctx, store.GetResearchScheduleParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Schedule{}, core.NotFound("research schedule %d not found", id)
	}
	if err != nil {
		return Schedule{}, err
	}
	in.BriefID = cur.BriefID
	if err := validateSchedule(in); err != nil {
		return Schedule{}, err
	}
	row, err := r.db.Writer().UpdateResearchSchedule(ctx, store.UpdateResearchScheduleParams{Name: strings.TrimSpace(in.Name), Cadence: string(in.Cadence), Enabled: boolInt(in.Enabled), UpdatedAt: r.clock.Now().UTC(), ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return Schedule{}, err
	}
	return scheduleFromRow(row), nil
}

func (r *Research) ListSchedules(ctx context.Context, actor core.Actor) ([]Schedule, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := r.db.Reader().ListResearchSchedules(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Schedule, 0, len(rows))
	for _, row := range rows {
		out = append(out, scheduleFromRow(row))
	}
	return out, nil
}

func validateSchedule(in ScheduleInput) error {
	if strings.TrimSpace(in.Name) == "" || utf8.RuneCountInString(in.Name) > 120 {
		return core.Invalid("a name of up to 120 characters is required")
	}
	switch in.Cadence {
	case reports.Weekly, reports.Monthly, reports.Quarterly:
	default:
		return core.Invalid("cadence must be weekly, monthly or quarterly")
	}
	if in.BriefID == 0 {
		return core.Invalid("a brief is required")
	}
	return nil
}

// StartRefreshRun queues a re-read of every readable item for a period.
// One per brief per period start.
func (r *Research) StartRefreshRun(ctx context.Context, actor core.Actor, rs *runs.Runs, briefID int64, periodStart reporting.Day) (runs.Run, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return runs.Run{}, false, core.ErrForbidden
	}
	if _, err := r.brief(ctx, actor.WorkspaceID, briefID); err != nil {
		return runs.Run{}, false, err
	}
	key := fmt.Sprintf("brief:%d/refresh/%s", briefID, periodStart)
	return rs.Start(ctx, actor, runs.StartInput{Kind: ResearchRunKind, OccurrenceKey: key, Params: RunParams{BriefID: briefID, Mode: "refresh"}})
}

// RunDue advances every enabled schedule one step: start the period's
// refresh run if it hasn't been, or write the report once that run has
// ended. Each schedule ends up with one report per period.
func (r *Research) RunDue(ctx context.Context, rs *runs.Runs) (started, generated int, errs []error) {
	rows, err := r.db.Reader().ListEnabledResearchSchedulesAllWorkspaces(ctx)
	if err != nil {
		return 0, 0, []error{err}
	}
	for _, s := range rows {
		actor := core.SystemIn(core.ViaJob, s.WorkspaceID)
		from, to := reports.Period(reports.Cadence(s.Cadence), r.clock.Now())
		key := reportKey(s.BriefID, from, to)
		if _, err := r.db.Reader().GetResearchReportByOccurrence(ctx, key); err == nil {
			continue
		}
		run, created, err := r.StartRefreshRun(ctx, actor, rs, s.BriefID, from)
		if err != nil {
			errs = append(errs, fmt.Errorf("schedule %d: %w", s.ID, err))
			continue
		}
		if created {
			started++
			continue
		}
		switch run.State {
		case runs.StateSucceeded, runs.StatePartial:
			if _, made, err := r.GenerateReport(ctx, actor, s.BriefID, s.ID, from, to); err != nil {
				errs = append(errs, fmt.Errorf("schedule %d: %w", s.ID, err))
			} else if made {
				generated++
			}
		}
	}
	return started, generated, errs
}

func reportKey(briefID int64, from, to reporting.Day) string {
	return fmt.Sprintf("brief:%d/report/%s..%s", briefID, from, to)
}

// Market events.

var eventKinds = map[string]bool{"festival": true, "economic": true, "regulatory": true, "competitor": true, "other": true}

// RecordMarketEvent stores what a person says happened, as evidence. It is
// never derived: no calendar is built in, because which events matter to
// Reportage's buyers is the team's knowledge.
func (r *Research) RecordMarketEvent(ctx context.Context, actor core.Actor, in MarketEventInput) (MarketEvent, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return MarketEvent{}, core.ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 160 {
		return MarketEvent{}, core.Invalid("a name of up to 160 characters is required")
	}
	if !eventKinds[in.Kind] {
		return MarketEvent{}, core.Invalid("kind must be festival, economic, regulatory, competitor or other")
	}
	from, err := time.Parse(dateLayout, strings.TrimSpace(in.From))
	if err != nil {
		return MarketEvent{}, core.Invalid("from must be a date like 2027-02-08")
	}
	to, err := time.Parse(dateLayout, strings.TrimSpace(in.To))
	if err != nil || to.Before(from) {
		return MarketEvent{}, core.Invalid("to must be a date on or after from")
	}
	in.From, in.To = from.Format(dateLayout), to.Format(dateLayout)
	in.SourceURL, in.Note = strings.TrimSpace(in.SourceURL), strings.TrimSpace(in.Note)
	payload := map[string]any{"name": in.Name, "kind": in.Kind, "from": in.From, "to": in.To, "sourceUrl": in.SourceURL, "note": in.Note, "recordedBy": actor.UserID}
	ev, _, err := r.evidence.Record(ctx, actor, evidence.Observation{Source: "manual", Kind: "market_event", SourceRef: in.Name + "|" + in.From, Scope: "market", Class: evidence.Observed, Payload: payload, Completeness: evidence.Complete})
	if err != nil {
		return MarketEvent{}, err
	}
	return eventFromEvidence(ev), nil
}

func (r *Research) ListMarketEvents(ctx context.Context, actor core.Actor) ([]MarketEvent, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := r.db.Reader().ListCurrentEvidenceByKind(ctx, store.ListCurrentEvidenceByKindParams{WorkspaceID: actor.WorkspaceID, Source: "manual", Kind: "market_event"})
	if err != nil {
		return nil, err
	}
	out := make([]MarketEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, eventFromEvidence(evidenceFromRow(row)))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out, nil
}

func eventFromEvidence(ev evidence.Evidence) MarketEvent {
	var p struct {
		Name, Kind, From, To, SourceURL, Note string
		RecordedBy                            int64
	}
	_ = json.Unmarshal(ev.Payload, &p)
	e := MarketEvent{EvidenceID: ev.ID, Name: p.Name, Kind: p.Kind, From: p.From, To: p.To, SourceURL: p.SourceURL, Note: p.Note, RecordedAt: ev.ObservedAt}
	if p.RecordedBy != 0 {
		e.RecordedBy = &p.RecordedBy
	}
	return e
}

// Reports.

// GenerateReport writes the comparative report for a brief and period. The
// occurrence is the brief and the period; asking twice returns the first
// report with created=false. The previous report for the brief, if any,
// supplies the prior context.
func (r *Research) GenerateReport(ctx context.Context, actor core.Actor, briefID, scheduleID int64, from, to reporting.Day) (ComparativeReport, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return ComparativeReport{}, false, core.ErrForbidden
	}
	ft, err1 := from.Time()
	tt, err2 := to.Time()
	if err1 != nil || err2 != nil || tt.Before(ft) {
		return ComparativeReport{}, false, core.Invalid("from and to must be YYYY-MM-DD with from on or before to")
	}
	brief, err := r.brief(ctx, actor.WorkspaceID, briefID)
	if err != nil {
		return ComparativeReport{}, false, err
	}
	key := reportKey(briefID, from, to)
	if existing, err := r.db.Reader().GetResearchReportByOccurrence(ctx, key); err == nil {
		rep, err := r.reportFromRow(ctx, existing)
		return rep, false, err
	}
	doc := ReportDocument{Period: DateRange{string(from), string(to)}, Competitors: []CompetitorComparison{}, Keywords: []KeywordComparison{}, Events: []MarketEvent{}, Hypotheses: []Hypothesis{}}
	days := int(tt.Sub(ft).Hours()/24) + 1
	doc.Previous = DateRange{string(reporting.DayOf(ft.AddDate(0, 0, -days))), string(reporting.DayOf(ft.AddDate(0, 0, -1)))}

	limited, reason := false, ""
	var accountEvidence int64
	if r.analytics == nil {
		limited, reason = true, "analytics are not configured on this server"
	} else {
		res, err := r.analytics.Analytics(ctx, actor, reporting.AnalyticsQuery{From: from, To: to, ProjectID: brief.ProjectID.Int64, GroupBy: "platform", Compare: "previous"})
		if err != nil {
			return ComparativeReport{}, false, err
		}
		if hasFigures(res) {
			ev, _, err := r.evidence.Record(ctx, actor, evidence.Observation{Source: "adx", Kind: "comparative_account", SourceRef: key, Scope: fmt.Sprintf("brief:%d", briefID), Class: evidence.Observed, Payload: res, Completeness: completenessOf(res)})
			if err != nil {
				return ComparativeReport{}, false, err
			}
			accountEvidence = ev.ID
			doc.Account = &AccountComparison{Summary: res.Summary, Rows: res.Rows, Freshness: res.Freshness, Coverage: res.Coverage, Warnings: res.Warnings, FormulaVersion: res.FormulaVersion, EvidenceID: ev.ID}
		} else {
			limited, reason = true, fmt.Sprintf("no account history for this scope in %s..%s or the period before; no trend is stated", from, to)
		}
	}

	items, err := r.db.Reader().ListResearchItems(ctx, briefID)
	if err != nil {
		return ComparativeReport{}, false, err
	}
	byBrand := map[int64]*CompetitorComparison{}
	var brandOrder []int64
	for _, it := range items {
		switch ItemKind(it.Kind) {
		case ItemCompetitorPage:
			if !it.CompetitorBrandID.Valid || strings.TrimSpace(it.Target) == "" {
				continue
			}
			versions, err := r.db.Reader().ListEvidenceVersions(ctx, store.ListEvidenceVersionsParams{WorkspaceID: actor.WorkspaceID, Source: "web", Kind: "page", SourceRef: it.Target, Limit: 2})
			if err != nil {
				return ComparativeReport{}, false, err
			}
			if len(versions) == 0 {
				continue
			}
			pc := pageComparison(it.ID, versions)
			cc, ok := byBrand[it.CompetitorBrandID.Int64]
			if !ok {
				b, err := r.db.Reader().GetBrand(ctx, store.GetBrandParams{ID: it.CompetitorBrandID.Int64, WorkspaceID: actor.WorkspaceID})
				name := it.CompetitorName
				if err == nil {
					name = b.Name
				}
				cc = &CompetitorComparison{BrandID: it.CompetitorBrandID.Int64, Name: name, Pages: []PageComparison{}}
				byBrand[it.CompetitorBrandID.Int64] = cc
				brandOrder = append(brandOrder, it.CompetitorBrandID.Int64)
			}
			cc.Pages = append(cc.Pages, pc)
		case ItemKeywords:
			if !brief.GoogleAdsAccountID.Valid {
				continue
			}
			acct, err := r.db.Reader().GetAccountByID(ctx, brief.GoogleAdsAccountID.Int64)
			if err != nil {
				continue
			}
			seeds := strings.Split(it.Target, "\n")
			ref := fmt.Sprintf("%s/%s/%s/%s", acct.Account.ExternalID, brief.Geography, brief.Language, strings.Join(seeds, "|"))
			versions, err := r.db.Reader().ListEvidenceVersions(ctx, store.ListEvidenceVersionsParams{WorkspaceID: actor.WorkspaceID, Source: "google_ads", Kind: "keyword_ideas", SourceRef: ref, Limit: 2})
			if err != nil {
				return ComparativeReport{}, false, err
			}
			if len(versions) > 0 {
				doc.Keywords = append(doc.Keywords, keywordComparisons(versions)...)
			}
		}
	}
	for _, id := range brandOrder {
		doc.Competitors = append(doc.Competitors, *byBrand[id])
	}
	events, err := r.ListMarketEvents(ctx, actor)
	if err != nil {
		return ComparativeReport{}, false, err
	}
	for _, e := range events {
		if e.From <= string(to) && e.To >= string(from) {
			doc.Events = append(doc.Events, e)
		}
	}
	doc.Hypotheses = hypotheses(doc, accountEvidence, limited)
	doc.Findings = []learning.MemoryFinding{}
	if r.memory != nil {
		if f, err := r.memory.Findings(ctx, actor); err == nil && f != nil {
			doc.Findings = f
		}
	}

	var prevID sql.NullInt64
	if prev, err := r.db.Reader().GetPreviousResearchReport(ctx, store.GetPreviousResearchReportParams{BriefID: briefID, PeriodStart: ft}); err == nil {
		prevID = sql.NullInt64{Int64: prev.ID, Valid: true}
		prior, err := r.priorContext(ctx, prev)
		if err != nil {
			return ComparativeReport{}, false, err
		}
		doc.Prior = &prior
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ComparativeReport{}, false, err
	}
	raw, _ := json.Marshal(doc)
	row, err := r.db.Writer().CreateResearchReport(ctx, store.CreateResearchReportParams{
		WorkspaceID: actor.WorkspaceID, BriefID: briefID, ScheduleID: optionalID(scheduleID), OccurrenceKey: key, PeriodStart: ft, PeriodEnd: tt, PreviousReportID: prevID,
		Limited: boolInt(limited), LimitedReason: reason, Document: string(raw), GeneratedAt: r.clock.Now().UTC(),
	})
	if store.IsUniqueViolation(err) {
		existing, err := r.db.Reader().GetResearchReportByOccurrence(ctx, key)
		if err != nil {
			return ComparativeReport{}, false, err
		}
		rep, err := r.reportFromRow(ctx, existing)
		return rep, false, err
	}
	if err != nil {
		return ComparativeReport{}, false, err
	}
	rep, err := r.reportFromRow(ctx, row)
	return rep, true, err
}

func hasFigures(res reporting.ReportingResult) bool {
	m := res.Summary.Metrics
	if m.Spend.Minor != nil || m.Leads > 0 || m.Impressions != nil || len(res.Rows) > 0 {
		return true
	}
	if p := res.Summary.Previous; p != nil && (p.Spend.Minor != nil || p.Leads > 0) {
		return true
	}
	return false
}

func completenessOf(res reporting.ReportingResult) evidence.Completeness {
	if res.Freshness.Partial {
		return evidence.Partial
	}
	return evidence.Complete
}

func pageComparison(itemID int64, versions []store.Evidence) PageComparison {
	latest := versions[0]
	var p struct {
		Title, Text string
	}
	_ = json.Unmarshal([]byte(latest.Payload), &p)
	pc := PageComparison{ItemID: itemID, URL: latest.SourceRef, Title: p.Title, EvidenceID: latest.ID, ExcerptAfter: excerpt(p.Text, 300), ObservedAt: latest.ObservedAt.UTC().Format(time.RFC3339)}
	if len(versions) > 1 {
		prev := versions[1]
		var q struct{ Text string }
		_ = json.Unmarshal([]byte(prev.Payload), &q)
		pid := prev.ID
		at := prev.ObservedAt.UTC().Format(time.RFC3339)
		pc.PreviousEvidenceID, pc.PreviousObservedAt = &pid, &at
		pc.ExcerptBefore = excerpt(q.Text, 300)
		pc.Changed = latest.PayloadHash != prev.PayloadHash
	}
	return pc
}

func keywordComparisons(versions []store.Evidence) []KeywordComparison {
	type ideas struct {
		Ideas []KeywordIdea `json:"ideas"`
	}
	var cur, prev ideas
	_ = json.Unmarshal([]byte(versions[0].Payload), &cur)
	before := map[string]*int64{}
	if len(versions) > 1 {
		_ = json.Unmarshal([]byte(versions[1].Payload), &prev)
		for _, k := range prev.Ideas {
			before[k.Text] = k.AvgMonthlySearches
		}
	}
	out := make([]KeywordComparison, 0, len(cur.Ideas))
	for _, k := range cur.Ideas {
		kc := KeywordComparison{Text: k.Text, After: k.AvgMonthlySearches, Class: string(evidence.Estimated), EvidenceID: versions[0].ID}
		if len(versions) > 1 {
			kc.Before = before[k.Text]
			if kc.Before != nil && kc.After != nil && *kc.Before > 0 {
				pct := math.Round((float64(*kc.After)-float64(*kc.Before))/float64(*kc.Before)*1000) / 10
				kc.ChangePct = &pct
			}
		}
		out = append(out, kc)
	}
	return out
}

// hypotheses is deterministic: one per recorded event overlapping the
// period, one per competitor page that changed, one per keyword estimate
// that moved by a fifth or more. Each cites the evidence it rests on. None
// claims a cause.
func hypotheses(doc ReportDocument, accountEvidence int64, limited bool) []Hypothesis {
	out := []Hypothesis{}
	observed := "No account history for this scope, so there is nothing to correlate with."
	if !limited && doc.Account != nil {
		observed = describeChange(doc.Account.Summary)
	}
	refs := func(ids ...int64) []int64 {
		out := []int64{}
		for _, id := range ids {
			if id != 0 {
				out = append(out, id)
			}
		}
		return out
	}
	for _, e := range doc.Events {
		out = append(out, Hypothesis{Key: fmt.Sprintf("event:%d", e.EvidenceID), Kind: "event", Observed: observed,
			Hypothetical: fmt.Sprintf("%s (%s, %s to %s) overlapped the period. A coincidence in time; whether it moved demand is not shown by these figures.", e.Name, e.Kind, e.From, e.To),
			EvidenceRefs: refs(e.EvidenceID, accountEvidence), Class: "hypothesis"})
	}
	for _, c := range doc.Competitors {
		for _, p := range c.Pages {
			if !p.Changed || p.PreviousEvidenceID == nil {
				continue
			}
			out = append(out, Hypothesis{Key: fmt.Sprintf("competitor:%d", p.ItemID), Kind: "competitor_change", Observed: observed,
				Hypothetical: fmt.Sprintf("%s changed the page %s since the last read. Whether that affected our results is not shown by these figures.", c.Name, p.URL),
				EvidenceRefs: refs(p.EvidenceID, *p.PreviousEvidenceID, accountEvidence), Class: "hypothesis"})
		}
	}
	for _, k := range doc.Keywords {
		if k.ChangePct == nil || math.Abs(*k.ChangePct) < 20 {
			continue
		}
		out = append(out, Hypothesis{Key: "demand:" + k.Text, Kind: "demand_shift", Observed: observed,
			Hypothetical: fmt.Sprintf("Google's estimated monthly searches for %q moved %+.0f%% (%d to %d) between runs. An estimate of demand, not a measurement, and not a cause.", k.Text, *k.ChangePct, *k.Before, *k.After),
			EvidenceRefs: refs(k.EvidenceID, accountEvidence), Class: "hypothesis"})
	}
	return out
}

func describeChange(s reporting.ReportRow) string {
	parts := []string{}
	part := func(name string, c reporting.Change, ok bool) {
		if !ok {
			return
		}
		if c.Undefined || c.Relative == nil {
			parts = append(parts, name+" change not available")
			return
		}
		parts = append(parts, fmt.Sprintf("%s %+.0f%%", name, *c.Relative*100))
	}
	spend, ok1 := s.Changes["spend"]
	part("spend", spend, ok1)
	leads, ok2 := s.Changes["leads"]
	part("leads", leads, ok2)
	cpl, ok3 := s.Changes["costPerLead"]
	part("cost per lead", cpl, ok3)
	if len(parts) == 0 {
		return "Figures for the period exist; no comparison with the previous period is available."
	}
	return "Against the previous period: " + strings.Join(parts, ", ") + "."
}

func (r *Research) priorContext(ctx context.Context, prev store.ResearchReport) (PriorContext, error) {
	var doc ReportDocument
	_ = json.Unmarshal([]byte(prev.Document), &doc)
	reviews, err := r.db.Reader().ListHypothesisReviews(ctx, prev.ID)
	if err != nil {
		return PriorContext{}, err
	}
	byKey := map[string]store.ResearchHypothesisReview{}
	for _, rv := range reviews {
		byKey[rv.HypothesisKey] = rv
	}
	pc := PriorContext{ReportID: prev.ID, PeriodStart: prev.PeriodStart.Format(dateLayout), PeriodEnd: prev.PeriodEnd.Format(dateLayout), Hypotheses: []PriorHypothesis{}}
	for _, h := range doc.Hypotheses {
		ph := PriorHypothesis{Key: h.Key, Kind: h.Kind, Text: h.Hypothetical}
		if rv, ok := byKey[h.Key]; ok {
			st := rv.Status
			ph.Status, ph.Note = &st, rv.Note
		}
		pc.Hypotheses = append(pc.Hypotheses, ph)
	}
	return pc, nil
}

// ReviewHypothesis records the team's judgement on one hypothesis. The
// hypothesis stays a hypothesis; "accepted" means the team believes it.
func (r *Research) ReviewHypothesis(ctx context.Context, actor core.Actor, reportID int64, key, status, note string) (ComparativeReport, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return ComparativeReport{}, core.ErrForbidden
	}
	switch status {
	case "accepted", "rejected", "corrected":
	default:
		return ComparativeReport{}, core.Invalid("status must be accepted, rejected or corrected")
	}
	note = strings.TrimSpace(note)
	if status == "corrected" && note == "" {
		return ComparativeReport{}, core.Invalid("a correction needs the corrected explanation in the note")
	}
	row, err := r.db.Reader().GetResearchReport(ctx, store.GetResearchReportParams{ID: reportID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return ComparativeReport{}, core.NotFound("research report %d not found", reportID)
	}
	if err != nil {
		return ComparativeReport{}, err
	}
	var doc ReportDocument
	_ = json.Unmarshal([]byte(row.Document), &doc)
	found := false
	for _, h := range doc.Hypotheses {
		if h.Key == key {
			found = true
			break
		}
	}
	if !found {
		return ComparativeReport{}, core.NotFound("report %d has no hypothesis %q", reportID, key)
	}
	if _, err := r.db.Writer().UpsertHypothesisReview(ctx, store.UpsertHypothesisReviewParams{WorkspaceID: actor.WorkspaceID, ReportID: reportID, HypothesisKey: key, Status: status, Note: note, DecidedBy: actor.UserID, DecidedAt: r.clock.Now().UTC()}); err != nil {
		return ComparativeReport{}, err
	}
	return r.reportFromRow(ctx, row)
}

func (r *Research) ListReports(ctx context.Context, actor core.Actor, briefID int64) ([]ComparativeReport, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if _, err := r.brief(ctx, actor.WorkspaceID, briefID); err != nil {
		return nil, err
	}
	rows, err := r.db.Reader().ListResearchReportsForBrief(ctx, store.ListResearchReportsForBriefParams{BriefID: briefID, Limit: 50})
	if err != nil {
		return nil, err
	}
	out := make([]ComparativeReport, 0, len(rows))
	for _, row := range rows {
		rep, err := r.reportFromRow(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, nil
}

func (r *Research) GetReport(ctx context.Context, actor core.Actor, id int64) (ComparativeReport, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ComparativeReport{}, core.ErrForbidden
	}
	row, err := r.db.Reader().GetResearchReport(ctx, store.GetResearchReportParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return ComparativeReport{}, core.NotFound("research report %d not found", id)
	}
	if err != nil {
		return ComparativeReport{}, err
	}
	return r.reportFromRow(ctx, row)
}

func (r *Research) reportFromRow(ctx context.Context, row store.ResearchReport) (ComparativeReport, error) {
	var doc ReportDocument
	if err := json.Unmarshal([]byte(row.Document), &doc); err != nil {
		return ComparativeReport{}, err
	}
	reviews, err := r.db.Reader().ListHypothesisReviews(ctx, row.ID)
	if err != nil {
		return ComparativeReport{}, err
	}
	rep := ComparativeReport{ID: row.ID, BriefID: row.BriefID, ScheduleID: idPtr(row.ScheduleID), OccurrenceKey: row.OccurrenceKey, PeriodStart: row.PeriodStart.Format(dateLayout), PeriodEnd: row.PeriodEnd.Format(dateLayout),
		PreviousReportID: idPtr(row.PreviousReportID), Limited: row.Limited == 1, LimitedReason: row.LimitedReason, Document: doc, Reviews: []HypothesisReview{}, GeneratedAt: row.GeneratedAt.UTC()}
	for _, rv := range reviews {
		rep.Reviews = append(rep.Reviews, HypothesisReview{Key: rv.HypothesisKey, Status: rv.Status, Note: rv.Note, DecidedBy: rv.DecidedBy, DecidedAt: rv.DecidedAt.UTC()})
	}
	return rep, nil
}

func scheduleFromRow(row store.ResearchSchedule) Schedule {
	return Schedule{ID: row.ID, BriefID: row.BriefID, Name: row.Name, Cadence: reports.Cadence(row.Cadence), Enabled: row.Enabled == 1, CreatedBy: idPtr(row.CreatedBy), CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// evidenceFromRow mirrors evidence.fromRow for the fields reports need.
func evidenceFromRow(r store.Evidence) evidence.Evidence {
	return evidence.Evidence{ID: r.ID, Source: r.Source, Kind: r.Kind, SourceRef: r.SourceRef, Scope: r.Scope, Class: evidence.Class(r.Class), ObservedAt: r.ObservedAt.UTC(), PayloadVersion: r.PayloadVersion, Payload: json.RawMessage(r.Payload), Completeness: evidence.Completeness(r.Completeness)}
}
