// Package reporting reads Google Ads and Meta performance into ADX and, in
// B1.3, computes the figures the screens show. This file is the read side:
// the provider interface core needs, the refresh run that walks an account's
// campaigns and days, and the freshness record every figure is labelled with.
//
// Google and Meta each claim their own conversions (reference.md), so rows
// carry their platform and are never summed across it. Money arrives in the
// account's currency and is stored in minor units. Days are the account's own
// days as the platform reports them; the Dubai reporting day is applied when
// figures are computed, not here.
package reporting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
)

// Day is a calendar day in the account's time zone, "YYYY-MM-DD".
type Day string

func DayOf(t time.Time) Day { return Day(t.Format("2006-01-02")) }

func (d Day) Time() (time.Time, error) { return time.Parse("2006-01-02", string(d)) }

// ProviderAccount is an ad account the credential can reach.
type ProviderAccount struct {
	ExternalID string
	Name       string
	Currency   string
	TimeZone   string
	// ParentID is the manager (Google) or business (Meta) above it.
	ParentID string
	Manager  bool
	Status   string
}

// Campaign is a campaign as the platform describes it.
type Campaign struct {
	ExternalID  string
	Name        string
	Status      string
	ChannelType string
	// BudgetMinor is the budget in minor units of Currency, or nil when the
	// platform gives none (Meta ad-set budgets live on the ad set).
	BudgetMinor  *int64
	BudgetPeriod string
	Currency     string
}

// DailyMetric is one campaign's one day as the platform reported it.
type DailyMetric struct {
	CampaignExternalID string
	Day                Day
	Currency           string
	CostMinor          int64
	Impressions        int64
	Clicks             int64
	Conversions        *float64
	ConversionsValue   *int64
	AllConversions     *float64
	// PlatformLeads is the platform's lead count (Google conversions of lead
	// type aren't separated; Meta's lead actions). Nil when not reported.
	PlatformLeads *float64
	Reach         *int64
}

type ConversionAction struct {
	ExternalID           string
	Name                 string
	Type                 string
	Category             string
	Status               string
	CountingType         string
	IncludeInConversions bool
}

// Page is a paginated result. Next is empty on the last page.
type Page[T any] struct {
	Items []T
	Next  string
}

// Provider is the read side of one ad platform, implemented in providers/
// and injected by cmd/adx. token is the credential's current access token.
type Provider interface {
	Platform() string
	ListAccounts(ctx context.Context, token string) ([]ProviderAccount, error)
	ListCampaigns(ctx context.Context, token, accountExternalID string, cursor string) (Page[Campaign], error)
	DailyMetrics(ctx context.Context, token, accountExternalID string, from, to Day, cursor string) (Page[DailyMetric], error)
	// ListConversionActions returns nil for platforms without them.
	ListConversionActions(ctx context.Context, token, accountExternalID string) ([]ConversionAction, error)
}

// Tokens gives the executor an account's current access token. core/connections
// implements it.
type Tokens interface {
	ProviderToken(ctx context.Context, actor core.Actor, accountID int64) (string, error)
}

// RefreshParams is an ads.refresh run.
type RefreshParams struct {
	AccountID int64 `json:"accountId"`
	From      Day   `json:"from"`
	To        Day   `json:"to"`
}

// RefreshProgress lets a retry resume after the campaigns and at the right
// metrics page.
type RefreshProgress struct {
	Stage           string `json:"stage"` // campaigns, metrics, actions, done
	Cursor          string `json:"cursor"`
	Campaigns       int    `json:"campaigns"`
	MetricRows      int    `json:"metricRows"`
	Pages           int    `json:"pages"`
	ThroughDay      Day    `json:"throughDay"`
	UnknownCampaign int    `json:"unknownCampaignRows"`
}

const RefreshKind = "ads.refresh"

// Freshness is how current an account's figures are.
type Freshness struct {
	AccountID       int64      `json:"accountId"`
	LastSuccessAt   *time.Time `json:"lastSuccessAt"`
	ObservedThrough *Day       `json:"observedThrough"`
	LastRunID       *int64     `json:"lastRunId"`
	LastPartial     bool       `json:"lastPartial"`
	LastError       *string    `json:"lastError"`
}

type Config struct{ Clock core.Clock }

type Reporting struct {
	db        *store.DB
	providers map[string]Provider
	tokens    Tokens
	evidence  *evidence.Store
	clock     core.Clock
}

// New wires the read side. providers is keyed by platform ("google_ads",
// "meta"); a missing platform makes its refresh fail with a clear message.
func New(db *store.DB, providers map[string]Provider, tokens Tokens, ev *evidence.Store, cfg Config) *Reporting {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if providers == nil {
		providers = map[string]Provider{}
	}
	return &Reporting{db: db, providers: providers, tokens: tokens, evidence: ev, clock: cfg.Clock}
}

// StartRefresh queues a read of one account's campaigns and daily metrics for
// a day range. The occurrence is the account plus the range plus the day it
// was asked, so one refresh per range per day is one run. Platforms revise
// recent days, so a daily job asks for the trailing 30 days.
func (r *Reporting) StartRefresh(ctx context.Context, actor core.Actor, rs *runs.Runs, accountID int64, from, to Day) (runs.Run, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return runs.Run{}, false, core.ErrForbidden
	}
	ft, err := from.Time()
	tt, err2 := to.Time()
	if err != nil || err2 != nil || tt.Before(ft) {
		return runs.Run{}, false, core.Invalid("from and to must be YYYY-MM-DD with from on or before to")
	}
	if tt.Sub(ft) > 400*24*time.Hour {
		return runs.Run{}, false, core.Invalid("a refresh covers at most 400 days; backfill in slices")
	}
	acct, err := r.db.Reader().GetAccountByID(ctx, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return runs.Run{}, false, core.NotFound("account %d is not connected", accountID)
	}
	if err != nil {
		return runs.Run{}, false, err
	}
	if _, ok := r.providers[acct.Account.Type]; !ok && acct.Account.Type != "google_ads" && acct.Account.Type != "meta" {
		return runs.Run{}, false, core.Invalid("account %d is not an ad account", accountID)
	}
	key := fmt.Sprintf("%s:%s/%s..%s/%s", acct.Account.Type, acct.Account.ExternalID, from, to, r.clock.Now().UTC().Format("2006-01-02"))
	return rs.Start(ctx, actor, runs.StartInput{Kind: RefreshKind, OccurrenceKey: key, AccountID: accountID,
		Params: RefreshParams{AccountID: accountID, From: from, To: to}})
}

// Execute works a refresh run: campaigns first (so metric rows have a
// campaign to hang on), then daily metrics page by page, then conversion
// actions. Progress after every page. A page that fails ends the run as
// failed with everything before it kept; the retry resumes at the cursor.
func (r *Reporting) Execute(ctx context.Context, h *runs.Handle) (runs.Completion, error) {
	var params RefreshParams
	if err := json.Unmarshal(h.Run.Params, &params); err != nil {
		return runs.Completion{}, core.Invalid("refresh run has invalid parameters")
	}
	acct, err := r.db.Reader().GetAccountByID(ctx, params.AccountID)
	if err != nil {
		return runs.Completion{}, err
	}
	provider, ok := r.providers[acct.Account.Type]
	if !ok || r.tokens == nil {
		return runs.Completion{}, core.Conflict("%s is not configured on this server", acct.Account.Type)
	}
	token, err := r.tokens.ProviderToken(ctx, h.Actor, params.AccountID)
	if err != nil {
		return runs.Completion{}, err
	}
	var progress RefreshProgress
	if len(h.Run.Progress) > 0 {
		_ = json.Unmarshal(h.Run.Progress, &progress)
	}
	if progress.Stage == "" {
		progress.Stage = "campaigns"
	}
	fail := func(err error) (runs.Completion, error) {
		_ = r.recordFreshness(ctx, h.Actor.WorkspaceID, params.AccountID, nil, "", h.Run.RowID(), true, safeMessage(err))
		return runs.Completion{}, err
	}
	currency := acct.Account.Currency.String

	if progress.Stage == "campaigns" {
		for {
			page, err := provider.ListCampaigns(ctx, token, acct.Account.ExternalID, progress.Cursor)
			if err != nil {
				return fail(err)
			}
			for _, c := range page.Items {
				if err := r.upsertCampaign(ctx, h, params.AccountID, acct.Account.Type, currency, c); err != nil {
					return fail(err)
				}
			}
			progress.Campaigns += len(page.Items)
			progress.Pages++
			progress.Cursor = page.Next
			if err := h.Progress(ctx, progress, nil); err != nil {
				return runs.Completion{}, err
			}
			if page.Next == "" {
				break
			}
		}
		progress.Stage, progress.Cursor = "metrics", ""
	}

	if progress.Stage == "metrics" {
		for {
			page, err := provider.DailyMetrics(ctx, token, acct.Account.ExternalID, params.From, params.To, progress.Cursor)
			if err != nil {
				return fail(err)
			}
			if err := r.storeMetrics(ctx, h, params, acct.Account.Type, page.Items, &progress); err != nil {
				return fail(err)
			}
			progress.Pages++
			progress.Cursor = page.Next
			if err := h.Progress(ctx, progress, nil); err != nil {
				return runs.Completion{}, err
			}
			if page.Next == "" {
				break
			}
		}
		progress.Stage, progress.Cursor = "actions", ""
	}

	if progress.Stage == "actions" {
		actions, err := provider.ListConversionActions(ctx, token, acct.Account.ExternalID)
		if err != nil {
			return fail(err)
		}
		now := r.clock.Now().UTC()
		for _, a := range actions {
			if err := r.db.Writer().UpsertConversionAction(ctx, store.UpsertConversionActionParams{
				WorkspaceID: h.Actor.WorkspaceID, AccountID: params.AccountID, ExternalID: a.ExternalID, Name: a.Name,
				Type: nullString(a.Type), Category: nullString(a.Category), Status: nullString(a.Status), CountingType: nullString(a.CountingType),
				IncludeInConversions: boolInt(a.IncludeInConversions), LastSeenAt: now,
			}); err != nil {
				return fail(err)
			}
		}
		progress.Stage = "done"
	}

	// The read is complete through the requested end day only if the end day
	// is in the past for the account; today is always partial.
	through := params.To
	partial := false
	if acct.Account.TimeZone.Valid {
		if loc, err := time.LoadLocation(acct.Account.TimeZone.String); err == nil {
			today := DayOf(r.clock.Now().In(loc))
			if params.To >= today {
				partial = true
				if y := DayOf(r.clock.Now().In(loc).AddDate(0, 0, -1)); y < params.To {
					through = y
				}
			}
		}
	}
	progress.ThroughDay = through
	if err := h.Progress(ctx, progress, nil); err != nil {
		return runs.Completion{}, err
	}
	// Campaign names carry the project (reference.md); read them now so the
	// analytics screen groups fresh campaigns by project at once.
	if _, _, err := r.AssignProjects(ctx, h.Actor); err != nil {
		return runs.Completion{}, err
	}
	now := r.clock.Now().UTC()
	if err := r.recordFreshness(ctx, h.Actor.WorkspaceID, params.AccountID, &now, through, h.Run.RowID(), partial, ""); err != nil {
		return runs.Completion{}, err
	}
	if partial {
		return runs.Completion{State: runs.StatePartial, Completeness: runs.CompletenessPartial}, nil
	}
	return runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}, nil
}

func (r *Reporting) upsertCampaign(ctx context.Context, h *runs.Handle, accountID int64, platform, accountCurrency string, c Campaign) error {
	now := r.clock.Now().UTC()
	currency := c.Currency
	if currency == "" {
		currency = accountCurrency
	}
	_, err := r.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{
		WorkspaceID: h.Actor.WorkspaceID, AccountID: accountID, Platform: platform, ExternalID: c.ExternalID, Name: c.Name, Status: c.Status,
		ChannelType: nullString(c.ChannelType), BudgetMinor: nullInt(c.BudgetMinor), BudgetPeriod: nullString(c.BudgetPeriod),
		Currency: nullString(currency), FirstSeenAt: now, LastSeenAt: now,
	})
	if err != nil {
		return err
	}
	_, _, err = r.evidence.Record(ctx, h.Actor, evidence.Observation{
		RunID: h.Run.RowID(), AccountID: accountID, Source: platform, Kind: "campaign", SourceRef: c.ExternalID,
		Scope: fmt.Sprintf("account:%d", accountID), Class: evidence.Observed, Payload: c, Completeness: evidence.Complete,
	})
	return err
}

// storeMetrics writes one page of daily rows and one evidence row for the
// page. A row for a campaign the campaign read didn't return (removed, or
// filtered by the platform) is counted and skipped, never invented.
func (r *Reporting) storeMetrics(ctx context.Context, h *runs.Handle, params RefreshParams, platform string, rows []DailyMetric, progress *RefreshProgress) error {
	if len(rows) == 0 {
		return nil
	}
	now := r.clock.Now().UTC()
	first, last := rows[0].Day, rows[len(rows)-1].Day
	ev, _, err := r.evidence.Record(ctx, h.Actor, evidence.Observation{
		RunID: h.Run.RowID(), AccountID: params.AccountID, Source: platform, Kind: "campaign_daily_metrics",
		SourceRef: fmt.Sprintf("%d/%s..%s/page-%d", params.AccountID, params.From, params.To, progress.Pages),
		Scope:     fmt.Sprintf("account:%d/days:%s..%s", params.AccountID, first, last), Class: evidence.Observed,
		Payload: rows, Completeness: evidence.Complete,
	})
	if err != nil {
		return err
	}
	_ = ev
	return r.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		for _, m := range rows {
			campaign, err := q.GetAdCampaignByExternalID(ctx, store.GetAdCampaignByExternalIDParams{AccountID: params.AccountID, ExternalID: m.CampaignExternalID})
			if errors.Is(err, sql.ErrNoRows) {
				progress.UnknownCampaign++
				continue
			}
			if err != nil {
				return err
			}
			if err := q.UpsertCampaignDailyMetrics(ctx, store.UpsertCampaignDailyMetricsParams{
				WorkspaceID: h.Actor.WorkspaceID, AccountID: params.AccountID, CampaignID: campaign.ID, Platform: platform, Day: string(m.Day),
				Currency: m.Currency, CostMinor: m.CostMinor, Impressions: m.Impressions, Clicks: m.Clicks,
				Conversions: nullFloat(m.Conversions), ConversionsValueMinor: nullInt(m.ConversionsValue), AllConversions: nullFloat(m.AllConversions),
				PlatformLeads: nullFloat(m.PlatformLeads), Reach: nullInt(m.Reach), ObservedAt: now, RunID: sql.NullInt64{Int64: h.Run.RowID(), Valid: true},
			}); err != nil {
				return err
			}
			progress.MetricRows++
		}
		return nil
	})
}

func (r *Reporting) recordFreshness(ctx context.Context, workspaceID, accountID int64, success *time.Time, through Day, runID int64, partial bool, lastErr string) error {
	return r.db.Writer().UpsertAccountFreshness(ctx, store.UpsertAccountFreshnessParams{
		AccountID: accountID, WorkspaceID: workspaceID, LastSuccessAt: nullTimePtr(success), ObservedThrough: nullString(string(through)),
		LastRunID: sql.NullInt64{Int64: runID, Valid: runID != 0}, LastPartial: boolInt(partial), LastError: nullString(lastErr),
	})
}

// ListFreshness returns per-account freshness for the connections screen.
func (r *Reporting) ListFreshness(ctx context.Context, actor core.Actor) ([]Freshness, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := r.db.Reader().ListAccountFreshness(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Freshness, 0, len(rows))
	for _, row := range rows {
		f := Freshness{AccountID: row.AccountID, LastPartial: row.LastPartial == 1}
		if row.LastSuccessAt.Valid {
			t := row.LastSuccessAt.Time.UTC()
			f.LastSuccessAt = &t
		}
		if row.ObservedThrough.Valid {
			d := Day(row.ObservedThrough.String)
			f.ObservedThrough = &d
		}
		if row.LastRunID.Valid {
			f.LastRunID = &row.LastRunID.Int64
		}
		if row.LastError.Valid {
			f.LastError = &row.LastError.String
		}
		out = append(out, f)
	}
	return out, nil
}

func safeMessage(err error) string {
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Message
	}
	return "the provider read failed; see the server log"
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: strings.TrimSpace(s) != ""}
}
func nullInt(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}
func nullFloat(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}
func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}
func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// RefreshAll starts a trailing-window refresh for every connected ad
// account, as ADX itself. The daily job calls it. Accounts whose start fails
// are logged by the caller through the returned errors; the others proceed.
func (r *Reporting) RefreshAll(ctx context.Context, rs *runs.Runs, trailingDays int) (started int, errs []error) {
	rows, err := r.db.Reader().ListAllAccounts(ctx)
	if err != nil {
		return 0, []error{err}
	}
	now := r.clock.Now().UTC()
	for _, row := range rows {
		if _, ok := r.providers[row.Account.Type]; !ok {
			continue
		}
		// Each account refreshes as ADX in its own client's workspace.
		actor := core.SystemIn(core.ViaJob, row.Account.WorkspaceID)
		loc := time.UTC
		if row.Account.TimeZone.Valid {
			if l, err := time.LoadLocation(row.Account.TimeZone.String); err == nil {
				loc = l
			}
		}
		local := now.In(loc)
		to := DayOf(local)
		from := DayOf(local.AddDate(0, 0, -trailingDays))
		if _, _, err := r.StartRefresh(ctx, actor, rs, row.Account.ID, from, to); err != nil {
			errs = append(errs, fmt.Errorf("account %d: %w", row.Account.ID, err))
			continue
		}
		started++
	}
	return started, errs
}
