package reporting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/store"
)

// FormulaVersion names the definitions below. It changes when a definition
// changes, so a stored report can say which one it used.
const FormulaVersion = "v1"

// ReportingZone is the reporting day (CLAUDE.md): Dubai.
var ReportingZone = mustZone("Asia/Dubai")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// AnalyticsQuery is what a screen asks for.
type AnalyticsQuery struct {
	From, To  Day
	AccountID int64
	Platform  string
	ProjectID int64
	// GroupBy is campaign, project, platform or day.
	GroupBy string
	// Compare is "", previous, year or custom.
	Compare                string
	CompareFrom, CompareTo Day
}

// Money is a nullable amount in minor units. Nil Minor means not available.
type Money struct {
	Minor    *int64 `json:"minor"`
	Currency string `json:"currency"`
}

// Ratio is a nullable division. Undefined is true when the denominator was
// zero: "no leads, so no cost per lead", which is not the same as zero.
type Ratio struct {
	Value     *float64 `json:"value"`
	Undefined bool     `json:"undefined"`
}

// Change compares a value with the comparison period. Relative is null when
// the previous value was zero or missing; Undefined says so explicitly.
type Change struct {
	Absolute  *float64 `json:"absolute"`
	Relative  *float64 `json:"relative"`
	Undefined bool     `json:"undefined"`
}

// ReportMetrics is one row of deterministic figures. Google conversions and Meta
// leads are separate fields and never summed. Leads, qualified and deals
// come from GoHighLevel and are counted once whichever platform claims them.
type ReportMetrics struct {
	Spend                Money    `json:"spend"`
	Impressions          *int64   `json:"impressions"`
	Clicks               *int64   `json:"clicks"`
	GoogleConversions    *float64 `json:"googleConversions"`
	MetaLeads            *float64 `json:"metaLeads"`
	Leads                int64    `json:"leads"`
	QualifiedLeads       *int64   `json:"qualifiedLeads"`
	Deals                *int64   `json:"deals"`
	Revenue              Money    `json:"revenue"`
	CostPerLead          Ratio    `json:"costPerLead"`
	CostPerQualifiedLead Ratio    `json:"costPerQualifiedLead"`
	ROAS                 Ratio    `json:"roas"`
}

// ReportRow is one group's figures and, when asked, how they changed.
type ReportRow struct {
	Key        string            `json:"key"`
	Label      string            `json:"label"`
	Platform   string            `json:"platform"`
	CampaignID *int64            `json:"campaignId"`
	ProjectID  *int64            `json:"projectId"`
	Metrics    ReportMetrics     `json:"metrics"`
	Previous   *ReportMetrics    `json:"previous"`
	Changes    map[string]Change `json:"changes"`
}

type ReportPeriod struct {
	Start      Day               `json:"start"`
	End        Day               `json:"end"`
	TimeZone   string            `json:"timeZone"`
	Comparison *ReportComparison `json:"comparison"`
}

type ReportComparison struct {
	Start Day    `json:"start"`
	End   Day    `json:"end"`
	Kind  string `json:"kind"`
}

type ResultFreshness struct {
	LastSuccessAt   *time.Time `json:"lastSuccessAt"`
	ObservedThrough *Day       `json:"observedThrough"`
	Partial         bool       `json:"partial"`
	MissingPeriods  []string   `json:"missingPeriods"`
}

type ReportCoverage struct {
	EligibleCount       int64    `json:"eligibleCount"`
	MatchedCount        int64    `json:"matchedCount"`
	UnclassifiedCount   int64    `json:"unclassifiedCount"`
	WithStatus          int64    `json:"withStatus"`
	DependenciesMissing []string `json:"dependenciesMissing"`
}

type ReportWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ReportingResult is the common reporting response (spec section 9).
type ReportingResult struct {
	Scope          ReportScope     `json:"scope"`
	Period         ReportPeriod    `json:"period"`
	Freshness      ResultFreshness `json:"freshness"`
	Coverage       ReportCoverage  `json:"coverage"`
	Summary        ReportRow       `json:"summary"`
	Rows           []ReportRow     `json:"rows"`
	Warnings       []ReportWarning `json:"warnings"`
	FormulaVersion string          `json:"formulaVersion"`
	GroupBy        string          `json:"groupBy"`
}

type ReportScope struct {
	WorkspaceID int64  `json:"workspaceId"`
	AccountID   *int64 `json:"accountId"`
	Platform    string `json:"platform"`
	ProjectID   *int64 `json:"projectId"`
}

// Analytics answers a AnalyticsQuery. Everything is computed from stored rows; the
// summary and the detail rows come from the same aggregation so they
// reconcile by construction.
func (r *Reporting) Analytics(ctx context.Context, actor core.Actor, q AnalyticsQuery) (ReportingResult, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ReportingResult{}, core.ErrForbidden
	}
	from, to, err := dayRange(q.From, q.To)
	if err != nil {
		return ReportingResult{}, err
	}
	switch q.GroupBy {
	case "", "campaign":
		q.GroupBy = "campaign"
	case "project", "platform", "day":
	default:
		return ReportingResult{}, core.Invalid("group must be campaign, project, platform or day")
	}
	if q.Platform != "" && q.Platform != "google_ads" && q.Platform != "meta" {
		return ReportingResult{}, core.Invalid("platform must be google_ads or meta")
	}
	result := ReportingResult{FormulaVersion: FormulaVersion, GroupBy: q.GroupBy, Warnings: []ReportWarning{},
		Scope:  ReportScope{WorkspaceID: actor.WorkspaceID, Platform: q.Platform},
		Period: ReportPeriod{Start: q.From, End: q.To, TimeZone: ReportingZone.String()}}
	if q.AccountID != 0 {
		result.Scope.AccountID = &q.AccountID
	}
	if q.ProjectID != 0 {
		result.Scope.ProjectID = &q.ProjectID
	}

	current, err := r.aggregate(ctx, actor, q, from, to)
	if err != nil {
		return ReportingResult{}, err
	}
	result.Rows, result.Summary = current.rows, current.summary
	result.Coverage = current.coverage
	result.Freshness = current.freshness
	result.Warnings = append(result.Warnings, current.warnings...)

	if q.Compare != "" {
		cf, ct, kind, err := comparisonRange(q, from, to)
		if err != nil {
			return ReportingResult{}, err
		}
		prev, err := r.aggregate(ctx, actor, AnalyticsQuery{From: cf, To: ct, AccountID: q.AccountID, Platform: q.Platform, ProjectID: q.ProjectID, GroupBy: q.GroupBy}, mustDayTime(cf), mustDayTime(ct).AddDate(0, 0, 1))
		if err != nil {
			return ReportingResult{}, err
		}
		result.Period.Comparison = &ReportComparison{Start: cf, End: ct, Kind: kind}
		byKey := map[string]ReportRow{}
		for _, p := range prev.rows {
			byKey[p.Key] = p
		}
		for i := range result.Rows {
			if p, ok := byKey[result.Rows[i].Key]; ok {
				m := p.Metrics
				result.Rows[i].Previous = &m
				result.Rows[i].Changes = changes(result.Rows[i].Metrics, m)
			} else {
				result.Rows[i].Changes = changes(result.Rows[i].Metrics, ReportMetrics{})
			}
		}
		pm := prev.summary.Metrics
		result.Summary.Previous = &pm
		result.Summary.Changes = changes(result.Summary.Metrics, pm)
		if len(prev.freshness.MissingPeriods) > 0 || prev.freshness.Partial {
			result.Warnings = append(result.Warnings, ReportWarning{Code: "comparison_incomplete", Message: "the comparison period has days with no figures"})
		}
	}
	return result, nil
}

type aggregation struct {
	rows      []ReportRow
	summary   ReportRow
	coverage  ReportCoverage
	freshness ResultFreshness
	warnings  []ReportWarning
}

type bucket struct {
	row         ReportRow
	spend       int64
	currency    string
	mixed       bool
	impressions int64
	clicks      int64
	gConv       *float64
	mLeads      *float64
	leads       int64
	qualified   int64
	deals       int64
	hasMetrics  bool
}

func (r *Reporting) aggregate(ctx context.Context, actor core.Actor, q AnalyticsQuery, from, to time.Time) (aggregation, error) {
	var agg aggregation
	agg.freshness.MissingPeriods = []string{}
	agg.coverage.DependenciesMissing = []string{}
	fromDay, toDay := DayOf(from), DayOf(to.AddDate(0, 0, -1))
	rows, err := r.db.Reader().ListCampaignDailyMetrics(ctx, store.ListCampaignDailyMetricsParams{
		WorkspaceID: actor.WorkspaceID, FromDay: string(fromDay), ToDay: string(toDay),
		AccountID: optionalID(q.AccountID), Platform: nullString(q.Platform),
	})
	if err != nil {
		return agg, err
	}
	campaigns, err := r.db.Reader().ListAllAdCampaigns(ctx, actor.WorkspaceID)
	if err != nil {
		return agg, err
	}
	campaignByID := map[int64]store.AdCampaign{}
	campaignByRef := map[string]store.AdCampaign{}
	for _, c := range campaigns {
		campaignByID[c.ID] = c
		campaignByRef[c.ExternalID] = c
		campaignByRef[strings.ToLower(strings.TrimSpace(c.Name))] = c
	}

	// Time zones: a row's day is the account's day. The reporting day is
	// Dubai's. Accounts in Asia/Dubai map 1:1; others are taken by calendar
	// date with a warning, because re-cutting a day needs hourly data the
	// platforms don't give.
	accounts, err := r.db.Reader().ListAdAccounts(ctx)
	if err != nil {
		return agg, err
	}
	accountZone := map[int64]string{}
	for _, a := range accounts {
		accountZone[a.Account.ID] = a.Account.TimeZone.String
	}
	zoneWarned := map[int64]bool{}

	buckets := map[string]*bucket{}
	keyFor := func(platform string, campaign store.AdCampaign, day string) (string, ReportRow) {
		switch q.GroupBy {
		case "project":
			if campaign.ProjectID.Valid {
				k := fmt.Sprintf("project:%d", campaign.ProjectID.Int64)
				return k, ReportRow{Key: k, Label: fmt.Sprintf("Project %d", campaign.ProjectID.Int64), ProjectID: &campaign.ProjectID.Int64}
			}
			return "project:none", ReportRow{Key: "project:none", Label: "No project"}
		case "platform":
			return "platform:" + platform, ReportRow{Key: "platform:" + platform, Label: platform, Platform: platform}
		case "day":
			return "day:" + day, ReportRow{Key: "day:" + day, Label: day}
		}
		k := fmt.Sprintf("campaign:%d", campaign.ID)
		id := campaign.ID
		row := ReportRow{Key: k, Label: campaign.Name, Platform: platform, CampaignID: &id}
		if campaign.ProjectID.Valid {
			row.ProjectID = &campaign.ProjectID.Int64
		}
		return k, row
	}
	get := func(key string, row ReportRow) *bucket {
		b, ok := buckets[key]
		if !ok {
			b = &bucket{row: row}
			buckets[key] = b
		}
		return b
	}

	daysSeen := map[int64]map[string]bool{}
	for _, m := range rows {
		campaign := campaignByID[m.CampaignID]
		if q.ProjectID != 0 && (!campaign.ProjectID.Valid || campaign.ProjectID.Int64 != q.ProjectID) {
			continue
		}
		if tz := accountZone[m.AccountID]; tz != "" && tz != ReportingZone.String() && !zoneWarned[m.AccountID] {
			zoneWarned[m.AccountID] = true
			agg.warnings = append(agg.warnings, ReportWarning{Code: "account_timezone_differs", Message: fmt.Sprintf("account %d reports days in %s; they are shown by calendar date, not re-cut to Dubai", m.AccountID, tz)})
		}
		if daysSeen[m.AccountID] == nil {
			daysSeen[m.AccountID] = map[string]bool{}
		}
		daysSeen[m.AccountID][m.Day] = true
		key, row := keyFor(m.Platform, campaign, m.Day)
		b := get(key, row)
		b.hasMetrics = true
		if b.currency == "" {
			b.currency = m.Currency
		} else if b.currency != m.Currency {
			b.mixed = true
		}
		b.spend += m.CostMinor
		b.impressions += m.Impressions
		b.clicks += m.Clicks
		if m.Conversions.Valid {
			b.gConv = addF(b.gConv, m.Conversions.Float64)
		}
		if m.PlatformLeads.Valid {
			b.mLeads = addF(b.mLeads, m.PlatformLeads.Float64)
		}
	}

	// Missing days: for each ad account in scope, days in the range with no
	// rows at all. An account that reported nothing for a day is a gap, not
	// a zero.
	for _, a := range accounts {
		if q.AccountID != 0 && a.Account.ID != q.AccountID {
			continue
		}
		if q.Platform != "" && a.Account.Type != q.Platform {
			continue
		}
		seen := daysSeen[a.Account.ID]
		for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
			if !seen[string(DayOf(d))] {
				agg.freshness.MissingPeriods = append(agg.freshness.MissingPeriods, fmt.Sprintf("account %d: %s", a.Account.ID, DayOf(d)))
			}
		}
	}
	sort.Strings(agg.freshness.MissingPeriods)
	if len(agg.freshness.MissingPeriods) > 20 {
		agg.freshness.MissingPeriods = append(agg.freshness.MissingPeriods[:20], fmt.Sprintf("and %d more", len(agg.freshness.MissingPeriods)-20))
	}

	// Leads from GoHighLevel, by the Dubai day they arrived, matched to a
	// campaign through their attribution's campaign reference.
	leadRows, err := r.db.Reader().ListLeadsAddedBetween(ctx, store.ListLeadsAddedBetweenParams{WorkspaceID: actor.WorkspaceID, FromAt: sql.NullTime{Time: from.UTC(), Valid: true}, ToAt: sql.NullTime{Time: to.UTC(), Valid: true}})
	if err != nil {
		return agg, err
	}
	attrs, err := r.db.Reader().ListLeadAttributionsBetween(ctx, store.ListLeadAttributionsBetweenParams{WorkspaceID: actor.WorkspaceID, FromAt: sql.NullTime{Time: from.UTC(), Valid: true}, ToAt: sql.NullTime{Time: to.UTC(), Valid: true}})
	if err != nil {
		return agg, err
	}
	refByLead := map[int64]store.LeadAttribution{}
	for _, a := range attrs {
		if _, ok := refByLead[a.LeadID]; !ok && a.MatchedCampaignRef.Valid {
			refByLead[a.LeadID] = a
		}
	}
	funnelConfigured := true
	if _, err := r.db.Reader().GetCurrentFunnelMapping(ctx, actor.WorkspaceID); errors.Is(err, sql.ErrNoRows) {
		funnelConfigured = false
	} else if err != nil {
		return agg, err
	}
	for _, l := range leadRows {
		agg.coverage.EligibleCount++
		if l.StatusRaw.Valid {
			agg.coverage.WithStatus++
		}
		attr, ok := refByLead[l.ID]
		var campaign store.AdCampaign
		matched := false
		if ok {
			if c, found := campaignByRef[attr.MatchedCampaignRef.String]; found {
				campaign, matched = c, true
			} else if c, found := campaignByRef[strings.ToLower(strings.TrimSpace(attr.MatchedCampaignRef.String))]; found {
				campaign, matched = c, true
			}
		}
		if !matched {
			agg.coverage.UnclassifiedCount++
			continue
		}
		if q.AccountID != 0 && campaign.AccountID != q.AccountID {
			continue
		}
		if q.Platform != "" && campaign.Platform != q.Platform {
			continue
		}
		if q.ProjectID != 0 && (!campaign.ProjectID.Valid || campaign.ProjectID.Int64 != q.ProjectID) {
			continue
		}
		agg.coverage.MatchedCount++
		day := ""
		if l.ProviderAddedAt.Valid {
			day = string(DayOf(l.ProviderAddedAt.Time.In(ReportingZone)))
		}
		key, row := keyFor(campaign.Platform, campaign, day)
		b := get(key, row)
		b.leads++
		if l.Stage.Valid {
			if leads.QualifiedStages[leads.Stage(l.Stage.String)] {
				b.qualified++
			}
			if leads.Stage(l.Stage.String) == leads.StageWon {
				b.deals++
			}
		}
	}
	if !funnelConfigured {
		agg.coverage.DependenciesMissing = append(agg.coverage.DependenciesMissing, "funnel mapping (status to stage) not configured: qualified leads and deals are not available")
	}
	agg.coverage.DependenciesMissing = append(agg.coverage.DependenciesMissing, "Done-Deal value does not arrive from Akarati as a field (#35): revenue and ROAS are not available")

	// Freshness from the accounts in scope.
	fresh, err := r.db.Reader().ListAccountFreshness(ctx, actor.WorkspaceID)
	if err != nil {
		return agg, err
	}
	for _, f := range fresh {
		if q.AccountID != 0 && f.AccountID != q.AccountID {
			continue
		}
		if f.LastSuccessAt.Valid && (agg.freshness.LastSuccessAt == nil || f.LastSuccessAt.Time.After(*agg.freshness.LastSuccessAt)) {
			t := f.LastSuccessAt.Time.UTC()
			agg.freshness.LastSuccessAt = &t
		}
		if f.ObservedThrough.Valid {
			d := Day(f.ObservedThrough.String)
			if agg.freshness.ObservedThrough == nil || d < *agg.freshness.ObservedThrough {
				agg.freshness.ObservedThrough = &d
			}
		}
		if f.LastPartial == 1 {
			agg.freshness.Partial = true
		}
	}
	if agg.freshness.ObservedThrough != nil && *agg.freshness.ObservedThrough < toDay {
		agg.freshness.Partial = true
	}

	// Finish rows and the summary from the same buckets.
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	summary := &bucket{row: ReportRow{Key: "summary", Label: "All"}}
	for _, k := range keys {
		b := buckets[k]
		b.row.Metrics = b.metrics(funnelConfigured)
		if b.mixed {
			agg.warnings = append(agg.warnings, ReportWarning{Code: "mixed_currencies", Message: b.row.Label + " mixes currencies; spend is not available"})
		}
		agg.rows = append(agg.rows, b.row)
		summary.hasMetrics = summary.hasMetrics || b.hasMetrics
		if summary.currency == "" {
			summary.currency = b.currency
		} else if b.currency != "" && summary.currency != b.currency {
			summary.mixed = true
		}
		summary.mixed = summary.mixed || b.mixed
		summary.spend += b.spend
		summary.impressions += b.impressions
		summary.clicks += b.clicks
		if b.gConv != nil {
			summary.gConv = addF(summary.gConv, *b.gConv)
		}
		if b.mLeads != nil {
			summary.mLeads = addF(summary.mLeads, *b.mLeads)
		}
		summary.leads += b.leads
		summary.qualified += b.qualified
		summary.deals += b.deals
	}
	summary.row.Metrics = summary.metrics(funnelConfigured)
	if summary.mixed {
		agg.warnings = append(agg.warnings, ReportWarning{Code: "mixed_currencies", Message: "the selection mixes currencies; total spend is not available"})
	}
	agg.summary = summary.row
	if agg.rows == nil {
		agg.rows = []ReportRow{}
	}
	return agg, nil
}

// metrics turns a bucket into figures with the formula version's rules:
// spend is null when currencies mix; ratios are undefined on a zero
// denominator; qualified and deals are null without a funnel; revenue is
// null until Done-Deal values arrive.
func (b *bucket) metrics(funnelConfigured bool) ReportMetrics {
	m := ReportMetrics{Leads: b.leads, Revenue: Money{Currency: b.currency}}
	if b.hasMetrics && !b.mixed {
		spend := b.spend
		m.Spend = Money{Minor: &spend, Currency: b.currency}
		imp, clk := b.impressions, b.clicks
		m.Impressions, m.Clicks = &imp, &clk
	} else {
		m.Spend = Money{Currency: b.currency}
	}
	m.GoogleConversions, m.MetaLeads = b.gConv, b.mLeads
	if funnelConfigured {
		q, d := b.qualified, b.deals
		m.QualifiedLeads, m.Deals = &q, &d
	}
	m.CostPerLead = divide(m.Spend.Minor, float64(b.leads))
	if m.QualifiedLeads != nil {
		m.CostPerQualifiedLead = divide(m.Spend.Minor, float64(*m.QualifiedLeads))
	}
	// ROAS = revenue / spend; revenue is not available, so ROAS is null,
	// never 0.
	m.ROAS = Ratio{}
	return m
}

// divide returns minor/denominator as a float of minor units, undefined when
// the denominator is zero and null when the numerator is not available.
func divide(minor *int64, denominator float64) Ratio {
	if minor == nil {
		return Ratio{}
	}
	if denominator == 0 {
		return Ratio{Undefined: true}
	}
	v := float64(*minor) / denominator
	return Ratio{Value: &v}
}

// changes compares current with previous per metric. A relative change is
// null when the previous value was zero or missing (Undefined), which is
// the defined non-infinite representation.
func changes(cur, prev ReportMetrics) map[string]Change {
	out := map[string]Change{}
	out["spend"] = change(moneyF(cur.Spend), moneyF(prev.Spend))
	out["impressions"] = change(intF(cur.Impressions), intF(prev.Impressions))
	out["clicks"] = change(intF(cur.Clicks), intF(prev.Clicks))
	l1, l2 := float64(cur.Leads), float64(prev.Leads)
	out["leads"] = change(&l1, &l2)
	out["qualifiedLeads"] = change(intF(cur.QualifiedLeads), intF(prev.QualifiedLeads))
	out["deals"] = change(intF(cur.Deals), intF(prev.Deals))
	out["googleConversions"] = change(cur.GoogleConversions, prev.GoogleConversions)
	out["metaLeads"] = change(cur.MetaLeads, prev.MetaLeads)
	out["costPerLead"] = change(cur.CostPerLead.Value, prev.CostPerLead.Value)
	out["costPerQualifiedLead"] = change(cur.CostPerQualifiedLead.Value, prev.CostPerQualifiedLead.Value)
	return out
}

func change(cur, prev *float64) Change {
	if cur == nil || prev == nil {
		return Change{Undefined: true}
	}
	abs := *cur - *prev
	if *prev == 0 {
		return Change{Absolute: &abs, Undefined: true}
	}
	rel := abs / *prev
	return Change{Absolute: &abs, Relative: &rel}
}

func moneyF(m Money) *float64 {
	if m.Minor == nil {
		return nil
	}
	v := float64(*m.Minor)
	return &v
}

func intF(v *int64) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}

func addF(acc *float64, v float64) *float64 {
	if acc == nil {
		return &v
	}
	sum := *acc + v
	return &sum
}

// dayRange turns inclusive days into [from, to) instants in Dubai.
func dayRange(from, to Day) (time.Time, time.Time, error) {
	f, err := from.Time()
	t, err2 := to.Time()
	if err != nil || err2 != nil {
		return time.Time{}, time.Time{}, core.Invalid("from and to must be YYYY-MM-DD")
	}
	if t.Before(f) {
		return time.Time{}, time.Time{}, core.Invalid("from must be on or before to")
	}
	if t.Sub(f) > 400*24*time.Hour {
		return time.Time{}, time.Time{}, core.Invalid("a period covers at most 400 days")
	}
	start := time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, ReportingZone)
	end := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ReportingZone).AddDate(0, 0, 1)
	return start, end, nil
}

func mustDayTime(d Day) time.Time {
	t, _ := d.Time()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, ReportingZone)
}

// comparisonRange works out the comparison period: the same number of days
// ending the day before (previous), the same dates a year earlier (year),
// or the custom days given. A period that is partly before the data begins
// is still returned; its missing days are reported as missing.
func comparisonRange(q AnalyticsQuery, from, to time.Time) (Day, Day, string, error) {
	days := int(to.Sub(from).Hours() / 24)
	switch q.Compare {
	case "previous":
		end := from.AddDate(0, 0, -1)
		start := from.AddDate(0, 0, -days)
		return DayOf(start), DayOf(end), "previous", nil
	case "year":
		return DayOf(from.AddDate(-1, 0, 0)), DayOf(to.AddDate(0, 0, -1).AddDate(-1, 0, 0)), "year", nil
	case "custom":
		if _, _, err := dayRange(q.CompareFrom, q.CompareTo); err != nil {
			return "", "", "", err
		}
		return q.CompareFrom, q.CompareTo, "custom", nil
	}
	return "", "", "", core.Invalid("compare must be previous, year or custom")
}

// AssignProjects reads each campaign's project from its name: the project
// whose name appears in the campaign name (reference.md: most campaigns
// are for one project). A campaign naming two projects, or none, stays
// unassigned and is listed. Admins and jobs may run it.
func (r *Reporting) AssignProjects(ctx context.Context, actor core.Actor) (assigned int, unresolved []string, err error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return 0, nil, core.ErrForbidden
	}
	projects, err := r.db.Reader().ListProjects(ctx)
	if err != nil {
		return 0, nil, err
	}
	campaigns, err := r.db.Reader().ListAllAdCampaigns(ctx, actor.WorkspaceID)
	if err != nil {
		return 0, nil, err
	}
	for _, c := range campaigns {
		var matches []store.Project
		for _, p := range projects {
			if nameMentions(c.Name, p.Name) {
				matches = append(matches, p)
			}
		}
		if len(matches) != 1 {
			if !c.ProjectID.Valid {
				unresolved = append(unresolved, c.Name)
			}
			continue
		}
		if c.ProjectID.Valid && c.ProjectID.Int64 == matches[0].ID {
			continue
		}
		if err := r.db.Writer().SetAdCampaignProject(ctx, store.SetAdCampaignProjectParams{ProjectID: sql.NullInt64{Int64: matches[0].ID, Valid: true}, ID: c.ID}); err != nil {
			return assigned, unresolved, err
		}
		assigned++
	}
	sort.Strings(unresolved)
	return assigned, unresolved, nil
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

// nameMentions reports whether a campaign name names the project. Both are
// lowercased, punctuation becomes spaces, and a leading "the" is dropped from
// the project, because Reportage spells the same project both ways ("Wadi",
// "The Wadi"). The project's words must appear as whole words, in order.
func nameMentions(campaign, project string) bool {
	words := func(s string) []string {
		s = strings.ToLower(s)
		s = strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 {
				return r
			}
			return ' '
		}, s)
		return strings.Fields(s)
	}
	cw, pw := words(campaign), words(project)
	if len(pw) > 1 && pw[0] == "the" {
		pw = pw[1:]
	}
	if len(pw) == 0 || len(pw) > len(cw) {
		return false
	}
	for i := 0; i+len(pw) <= len(cw); i++ {
		match := true
		for j := range pw {
			if cw[i+j] != pw[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
