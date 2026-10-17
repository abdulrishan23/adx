package research_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/reports"
	"github.com/WokeWorks/adx/server/core/research"
	"github.com/WokeWorks/adx/server/core/runs"
)

// switchAnalytics returns whatever result it currently holds.
type switchAnalytics struct{ res reporting.ReportingResult }

func (s *switchAnalytics) Analytics(context.Context, core.Actor, reporting.AnalyticsQuery) (reporting.ReportingResult, error) {
	return s.res, nil
}

func withHistory() reporting.ReportingResult {
	spend, prevSpend := int64(2_000_000), int64(1_600_000)
	rel := 0.25
	leadsRel := -0.1
	return reporting.ReportingResult{FormulaVersion: "v1", GroupBy: "platform",
		Summary: reporting.ReportRow{Key: "all", Label: "all",
			Metrics:  reporting.ReportMetrics{Spend: reporting.Money{Minor: &spend, Currency: "AED"}, Leads: 45},
			Previous: &reporting.ReportMetrics{Spend: reporting.Money{Minor: &prevSpend, Currency: "AED"}, Leads: 50},
			Changes:  map[string]reporting.Change{"spend": {Relative: &rel}, "leads": {Relative: &leadsRel}, "costPerLead": {Undefined: true}}},
		Rows: []reporting.ReportRow{{Key: "google_ads", Label: "Google Ads", Platform: "google_ads", Metrics: reporting.ReportMetrics{Spend: reporting.Money{Minor: &spend, Currency: "AED"}, Leads: 45}}}}
}

// Catches: a report for a brand with no history stating a trend, a festival
// explanation appearing without a recorded event, and a recorded event
// being written up as a cause.
func TestNewBrandGetsLimitedAuditAndNoCausalClaims(t *testing.T) {
	e := setup(t, nil) // real reporting, empty database: no history
	ctx := t.Context()
	e.fetcher.pages["https://emaar.example/"] = research.Page{Title: "Emaar", Text: "Launch offer"}
	d, err := e.research.CreateBrief(ctx, coretest.Marketer, research.BriefInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Monthly", Competitors: []research.CompetitorInput{{Name: "Emaar", URLs: []string{"https://emaar.example/"}}}, AuditAccount: true})
	require.NoError(t, err)
	e.run(t, d.Brief.ID)

	rep, created, err := e.research.GenerateReport(ctx, coretest.Marketer, d.Brief.ID, 0, "2026-09-01", "2026-09-30")
	require.NoError(t, err)
	assert.True(t, created)
	assert.True(t, rep.Limited)
	assert.Contains(t, rep.LimitedReason, "no account history")
	assert.Nil(t, rep.Document.Account)
	assert.Equal(t, research.DateRange{From: "2026-08-02", To: "2026-08-31"}, rep.Document.Previous)
	assert.Empty(t, rep.Document.Hypotheses, "nothing to explain and nothing recorded")
	require.Len(t, rep.Document.Competitors, 1)
	assert.False(t, rep.Document.Competitors[0].Pages[0].Changed, "one read is not a change")
	again, created, err := e.research.GenerateReport(ctx, coretest.Marketer, d.Brief.ID, 0, "2026-09-01", "2026-09-30")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, rep.ID, again.ID)

	// A festival only appears once someone records it, and then as a hypothesis.
	_, err = e.research.RecordMarketEvent(ctx, coretest.Viewer, research.MarketEventInput{Name: "Eid", Kind: "festival", From: "2026-10-05", To: "2026-10-08"})
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.research.RecordMarketEvent(ctx, coretest.Marketer, research.MarketEventInput{Name: "Eid", Kind: "holiday", From: "2026-10-05", To: "2026-10-08"})
	require.Error(t, err, "unknown kind")
	ev, err := e.research.RecordMarketEvent(ctx, coretest.Marketer, research.MarketEventInput{Name: "Eid al-Etihad week", Kind: "festival", From: "2026-10-05", To: "2026-10-08", SourceURL: "https://u.ae/en/about-the-uae/holidays"})
	require.NoError(t, err)
	rep2, created, err := e.research.GenerateReport(ctx, coretest.Marketer, d.Brief.ID, 0, "2026-10-01", "2026-10-31")
	require.NoError(t, err)
	assert.True(t, created)
	assert.True(t, rep2.Limited)
	require.Len(t, rep2.Document.Events, 1)
	require.Len(t, rep2.Document.Hypotheses, 1)
	h := rep2.Document.Hypotheses[0]
	assert.Equal(t, "event", h.Kind)
	assert.Equal(t, "hypothesis", h.Class)
	assert.False(t, h.Causal)
	assert.Equal(t, []int64{ev.EvidenceID}, h.EvidenceRefs)
	assert.Contains(t, h.Observed, "No account history")
	assert.Contains(t, h.Hypothetical, "coincidence in time")
	require.NotNil(t, rep2.Document.Prior)
	assert.Equal(t, rep.ID, rep2.Document.Prior.ReportID)
	assert.Equal(t, rep.ID, *rep2.PreviousReportID)
}

// Catches: a changed competitor page or a moved keyword estimate not being
// surfaced, a hypothesis without evidence refs, an operator's acceptance
// turning a hypothesis into a fact, a viewer reviewing, a review of a
// hypothesis that doesn't exist, and the next report forgetting the review.
func TestHypothesesCiteEvidenceAndReviewsCarryForward(t *testing.T) {
	an := &switchAnalytics{res: withHistory()}
	e := setup(t, an)
	ctx := t.Context()
	e.fetcher.pages["https://emaar.example/"] = research.Page{Title: "Emaar", Text: "Pay 10% now, 90% on handover"}
	e.keywords.ideas = []research.KeywordIdea{{Text: "villas dubai", AvgMonthlySearches: i64(10000)}, {Text: "wadi", AvgMonthlySearches: i64(500)}}
	d, err := e.research.CreateBrief(ctx, coretest.Marketer, research.BriefInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Monthly", Competitors: []research.CompetitorInput{{Name: "Emaar", URLs: []string{"https://emaar.example/"}}},
		KeywordSeeds: []string{"villas dubai"}, GoogleAdsAccountID: e.googleAcc, AuditAccount: true})
	require.NoError(t, err)
	e.run(t, d.Brief.ID)

	// A month later the competitor changed its page and demand estimates moved.
	e.clock.Advance(30 * 24 * time.Hour)
	e.fetcher.pages["https://emaar.example/"] = research.Page{Title: "Emaar", Text: "Pay 5% now, 95% on handover, free DLD"}
	e.keywords.ideas = []research.KeywordIdea{{Text: "villas dubai", AvgMonthlySearches: i64(13000)}, {Text: "wadi", AvgMonthlySearches: i64(520)}}
	run, _, err := e.research.StartRefreshRun(ctx, coretest.Marketer, e.runs, d.Brief.ID, "2026-10-01")
	require.NoError(t, err)
	h, err := e.runs.Begin(ctx, run.RowID(), "test")
	require.NoError(t, err)
	completion, err := e.research.Execute(ctx, h)
	require.NoError(t, err)
	require.NoError(t, h.Finish(ctx, completion))
	assert.Equal(t, runs.StateSucceeded, completion.State)
	assert.Equal(t, 2, e.fetcher.calls["https://emaar.example/"], "refresh re-reads a fetched page")

	_, err = e.research.RecordMarketEvent(ctx, coretest.Marketer, research.MarketEventInput{Name: "Cityscape Global", Kind: "economic", From: "2026-10-20", To: "2026-10-22"})
	require.NoError(t, err)
	rep, created, err := e.research.GenerateReport(ctx, coretest.Marketer, d.Brief.ID, 0, "2026-10-01", "2026-10-31")
	require.NoError(t, err)
	require.True(t, created)
	assert.False(t, rep.Limited)
	require.NotNil(t, rep.Document.Account)
	assert.Equal(t, int64(45), rep.Document.Account.Summary.Metrics.Leads)
	page := rep.Document.Competitors[0].Pages[0]
	assert.True(t, page.Changed)
	assert.Contains(t, page.ExcerptBefore, "10% now")
	assert.Contains(t, page.ExcerptAfter, "5% now")
	require.NotNil(t, page.PreviousEvidenceID)
	kws := map[string]research.KeywordComparison{}
	for _, k := range rep.Document.Keywords {
		kws[k.Text] = k
	}
	assert.Equal(t, 30.0, *kws["villas dubai"].ChangePct)
	assert.Equal(t, "estimated", kws["villas dubai"].Class)
	assert.Equal(t, 4.0, *kws["wadi"].ChangePct)

	kinds := map[string]research.Hypothesis{}
	for _, hy := range rep.Document.Hypotheses {
		kinds[hy.Kind] = hy
		assert.Equal(t, "hypothesis", hy.Class)
		assert.False(t, hy.Causal)
		assert.NotEmpty(t, hy.EvidenceRefs, hy.Key)
		assert.Contains(t, hy.Observed, "spend +25%")
		assert.Contains(t, hy.Observed, "leads -10%")
		assert.Contains(t, hy.Observed, "cost per lead change not available")
	}
	assert.Len(t, kinds, 3, "event, competitor change, demand shift")
	assert.Contains(t, kinds["demand_shift"].Hypothetical, "+30%")
	assert.Contains(t, kinds["demand_shift"].Hypothetical, "estimate")
	assert.Contains(t, kinds["competitor_change"].EvidenceRefs, *page.PreviousEvidenceID)
	assert.NotContains(t, kinds, "demand_shift:wadi", "a 4% move is noise")

	// Review.
	_, err = e.research.ReviewHypothesis(ctx, coretest.Viewer, rep.ID, kinds["event"].Key, "accepted", "")
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.research.ReviewHypothesis(ctx, coretest.Marketer, rep.ID, "event:999", "accepted", "")
	require.Error(t, err)
	_, err = e.research.ReviewHypothesis(ctx, coretest.Marketer, rep.ID, kinds["event"].Key, "corrected", "")
	require.Error(t, err, "a correction needs text")
	reviewed, err := e.research.ReviewHypothesis(ctx, coretest.Marketer, rep.ID, kinds["event"].Key, "corrected", "Cityscape brought brokers, not buyers; leads were down because the Wadi launch moved to November.")
	require.NoError(t, err)
	require.Len(t, reviewed.Reviews, 1)
	assert.Equal(t, "corrected", reviewed.Reviews[0].Status)
	for _, hy := range reviewed.Document.Hypotheses {
		assert.Equal(t, "hypothesis", hy.Class, "a review doesn't change the class")
	}
	_, err = e.research.ReviewHypothesis(ctx, coretest.Admin, rep.ID, kinds["competitor_change"].Key, "accepted", "")
	require.NoError(t, err)

	// The next period's report carries the reviews forward.
	e.clock.Advance(30 * 24 * time.Hour)
	next, created, err := e.research.GenerateReport(ctx, coretest.Marketer, d.Brief.ID, 0, "2026-11-01", "2026-11-30")
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, next.Document.Prior)
	assert.Equal(t, rep.ID, next.Document.Prior.ReportID)
	statuses := map[string]string{}
	for _, ph := range next.Document.Prior.Hypotheses {
		if ph.Status != nil {
			statuses[ph.Kind] = *ph.Status + ":" + ph.Note
		} else {
			statuses[ph.Kind] = "unreviewed"
		}
	}
	assert.Equal(t, "corrected:Cityscape brought brokers, not buyers; leads were down because the Wadi launch moved to November.", statuses["event"])
	assert.Equal(t, "accepted:", statuses["competitor_change"])
	assert.Equal(t, "unreviewed", statuses["demand_shift"])
	list, err := e.research.ListReports(ctx, coretest.Viewer, d.Brief.ID)
	require.NoError(t, err)
	assert.Len(t, list, 2)
	got, err := e.research.GetReport(ctx, coretest.Viewer, rep.ID)
	require.NoError(t, err)
	assert.Len(t, got.Reviews, 2)
}

// Catches: a schedule writing a report before its refresh ran, a report
// being written twice for a period, a disabled schedule running, and a
// daily cadence being accepted.
func TestScheduleRefreshesThenReportsOncePerPeriod(t *testing.T) {
	e := setup(t, &switchAnalytics{res: withHistory()})
	ctx := t.Context()
	e.fetcher.pages["https://market.example/"] = research.Page{Title: "Market", Text: "Q3 transactions"}
	d, err := e.research.CreateBrief(ctx, coretest.Marketer, research.BriefInput{BrandID: e.reportage, Title: "Market", URLs: []string{"https://market.example/"}, AuditAccount: true})
	require.NoError(t, err)
	_, err = e.research.CreateSchedule(ctx, coretest.Marketer, research.ScheduleInput{BriefID: d.Brief.ID, Name: "Monthly market", Cadence: reports.Daily, Enabled: true})
	require.Error(t, err, "daily is not a research cadence")
	_, err = e.research.CreateSchedule(ctx, coretest.Viewer, research.ScheduleInput{BriefID: d.Brief.ID, Name: "x", Cadence: reports.Monthly, Enabled: true})
	assert.ErrorIs(t, err, core.ErrForbidden)
	sched, err := e.research.CreateSchedule(ctx, coretest.Marketer, research.ScheduleInput{BriefID: d.Brief.ID, Name: "Monthly market", Cadence: reports.Monthly, Enabled: true})
	require.NoError(t, err)
	off, err := e.research.CreateSchedule(ctx, coretest.Marketer, research.ScheduleInput{BriefID: d.Brief.ID, Name: "Paused", Cadence: reports.Weekly, Enabled: false})
	require.NoError(t, err)

	started, generated, errs := e.research.RunDue(ctx, e.runs)
	assert.Empty(t, errs)
	assert.Equal(t, 1, started)
	assert.Equal(t, 0, generated, "no report before the refresh ran")
	started, generated, _ = e.research.RunDue(ctx, e.runs)
	assert.Equal(t, 0, started, "the refresh is queued; not started again")
	assert.Equal(t, 0, generated)

	// Work the queued run as the job worker would.
	all, err := e.runs.List(ctx, coretest.Viewer, 10)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Contains(t, all[0].OccurrenceKey, "/refresh/2026-09-01")
	h, err := e.runs.Begin(ctx, all[0].RowID(), "test")
	require.NoError(t, err)
	completion, err := e.research.Execute(ctx, h)
	require.NoError(t, err)
	require.NoError(t, h.Finish(ctx, completion))

	started, generated, errs = e.research.RunDue(ctx, e.runs)
	assert.Empty(t, errs)
	assert.Equal(t, 0, started)
	assert.Equal(t, 1, generated)
	started, generated, _ = e.research.RunDue(ctx, e.runs)
	assert.Equal(t, 0, started+generated, "one report per period")
	list, err := e.research.ListReports(ctx, coretest.Viewer, d.Brief.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "2026-09-01", list[0].PeriodStart)
	assert.Equal(t, "2026-09-30", list[0].PeriodEnd)
	assert.Equal(t, sched.ID, *list[0].ScheduleID)

	// Disabling stops it; the paused one never ran.
	_, err = e.research.UpdateSchedule(ctx, coretest.Marketer, sched.ID, research.ScheduleInput{Name: "Monthly market", Cadence: reports.Monthly, Enabled: false})
	require.NoError(t, err)
	e.clock.Advance(31 * 24 * time.Hour)
	started, generated, _ = e.research.RunDue(ctx, e.runs)
	assert.Equal(t, 0, started+generated)
	schedules, err := e.research.ListSchedules(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Len(t, schedules, 2)
	_ = off
}
