// Package alerts runs the anomaly checks (reference.md: cost spikes, zero-lead
// days, stale ingestion) over the deterministic reporting results and keeps
// what they found. Which checks run and at what thresholds is business
// policy, so the configuration ships empty: no check fires until an admin
// sets its threshold. A check that fires again for the same subject while
// its alert is open adds an occurrence instead of a new alert. Alerts change
// nothing in any ad account.
package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

// Check names.
const (
	CheckCostSpike      = "cost_spike"
	CheckZeroLeads      = "zero_leads"
	CheckStaleIngestion = "stale_ingestion"
)

// CostSpikeConfig fires when a campaign's spend on the day is more than
// ThresholdPct above its average over the days it ran in the baseline
// window, with at least MinSpendMinor spent and at least MinBaselineDays of
// figures to average (a campaign that ran one day has no baseline). Nil
// threshold means off.
type CostSpikeConfig struct {
	ThresholdPct    *int   `json:"thresholdPct"`
	MinSpendMinor   *int64 `json:"minSpendMinor"`
	BaselineDays    int    `json:"baselineDays"`
	MinBaselineDays int    `json:"minBaselineDays"`
}

// ZeroLeadsConfig fires when a campaign spent at least MinSpendMinor on a
// day the figures are complete for and no lead arrived. Nil means off.
type ZeroLeadsConfig struct {
	MinSpendMinor *int64 `json:"minSpendMinor"`
}

// StaleIngestionConfig fires when an account's figures are older than
// MaxAgeHours. Nil means off.
type StaleIngestionConfig struct {
	MaxAgeHours *int `json:"maxAgeHours"`
}

type AlertDocument struct {
	CostSpike      CostSpikeConfig      `json:"costSpike"`
	ZeroLeads      ZeroLeadsConfig      `json:"zeroLeads"`
	StaleIngestion StaleIngestionConfig `json:"staleIngestion"`
	Note           string               `json:"note"`
}

type AlertConfig struct {
	Version   int64         `json:"version"`
	Document  AlertDocument `json:"document"`
	CreatedBy *int64        `json:"createdBy"`
	CreatedAt time.Time     `json:"createdAt"`
}

type AlertConfigState struct {
	Configured   bool         `json:"configured"`
	Current      *AlertConfig `json:"current"`
	Unconfigured []string     `json:"unconfigured"`
}

type Alert struct {
	ID             int64           `json:"id"`
	Check          string          `json:"check"`
	SubjectKey     string          `json:"subjectKey"`
	SubjectLabel   string          `json:"subjectLabel"`
	Severity       string          `json:"severity"`
	State          string          `json:"state"`
	Message        string          `json:"message"`
	Evidence       json.RawMessage `json:"evidence"`
	ConfigVersion  int64           `json:"configVersion"`
	Occurrences    int64           `json:"occurrences"`
	FirstSeenAt    time.Time       `json:"firstSeenAt"`
	LastSeenAt     time.Time       `json:"lastSeenAt"`
	AcknowledgedBy *int64          `json:"acknowledgedBy"`
	AcknowledgedAt *time.Time      `json:"acknowledgedAt"`
	ResolvedAt     *time.Time      `json:"resolvedAt"`
}

// AlertEvaluation is what one run of the checks did.
type AlertEvaluation struct {
	Day       reporting.Day `json:"day"`
	Fired     int           `json:"fired"`
	Repeated  int           `json:"repeated"`
	Skipped   []string      `json:"skipped"`
	Evaluated bool          `json:"evaluated"`
}

type Config struct{ Clock core.Clock }

type Alerts struct {
	db        *store.DB
	reporting *reporting.Reporting
	clock     core.Clock
}

func New(db *store.DB, r *reporting.Reporting, cfg Config) *Alerts {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Alerts{db: db, reporting: r, clock: cfg.Clock}
}

func (a *Alerts) GetConfig(ctx context.Context, actor core.Actor) (AlertConfigState, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return AlertConfigState{}, core.ErrForbidden
	}
	cur, err := a.current(ctx, actor.WorkspaceID)
	if err != nil {
		return AlertConfigState{}, err
	}
	state := AlertConfigState{Unconfigured: []string{}}
	if cur != nil {
		state.Configured, state.Current = true, cur
	}
	state.Unconfigured = unconfigured(cur)
	return state, nil
}

func (a *Alerts) PutConfig(ctx context.Context, actor core.Actor, doc AlertDocument) (AlertConfig, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return AlertConfig{}, core.ErrForbidden
	}
	if doc.CostSpike.ThresholdPct != nil && (*doc.CostSpike.ThresholdPct < 1 || *doc.CostSpike.ThresholdPct > 10000) {
		return AlertConfig{}, core.Invalid("cost spike threshold must be between 1 and 10000 percent")
	}
	if doc.CostSpike.BaselineDays < 0 || doc.CostSpike.BaselineDays > 90 {
		return AlertConfig{}, core.Invalid("baseline days must be between 0 and 90")
	}
	if doc.CostSpike.BaselineDays == 0 {
		doc.CostSpike.BaselineDays = 7
	}
	if doc.CostSpike.MinBaselineDays < 0 || doc.CostSpike.MinBaselineDays > doc.CostSpike.BaselineDays {
		return AlertConfig{}, core.Invalid("minimum baseline days must be between 0 and the baseline days")
	}
	if doc.CostSpike.MinBaselineDays == 0 {
		doc.CostSpike.MinBaselineDays = 3
	}
	for _, v := range []*int64{doc.CostSpike.MinSpendMinor, doc.ZeroLeads.MinSpendMinor} {
		if v != nil && *v < 0 {
			return AlertConfig{}, core.Invalid("minimum spend must not be negative")
		}
	}
	if doc.StaleIngestion.MaxAgeHours != nil && (*doc.StaleIngestion.MaxAgeHours < 1 || *doc.StaleIngestion.MaxAgeHours > 24*30) {
		return AlertConfig{}, core.Invalid("stale ingestion age must be between 1 and 720 hours")
	}
	raw, _ := json.Marshal(doc)
	var row store.AlertConfig
	err := a.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		version := int64(1)
		if cur, err := q.GetCurrentAlertConfig(ctx, actor.WorkspaceID); err == nil {
			version = cur.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var err error
		row, err = q.CreateAlertConfig(ctx, store.CreateAlertConfigParams{WorkspaceID: actor.WorkspaceID, Version: version, Document: string(raw),
			CreatedBy: optionalID(actor.UserID), CreatedAt: a.clock.Now().UTC()})
		return err
	})
	if store.IsUniqueViolation(err) {
		return AlertConfig{}, core.Conflict("someone saved the alert configuration at the same time; reload and try again")
	}
	if err != nil {
		return AlertConfig{}, err
	}
	return configFromRow(row)
}

// Evaluate runs the configured checks for one Dubai day. It is honest about
// eligibility: a day whose figures are not complete raises stale ingestion
// (if configured) and never a zero-lead alert, because no figures is not
// zero leads.
func (a *Alerts) Evaluate(ctx context.Context, actor core.Actor, day reporting.Day) (AlertEvaluation, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return AlertEvaluation{}, core.ErrForbidden
	}
	ev := AlertEvaluation{Day: day, Skipped: []string{}}
	cur, err := a.current(ctx, actor.WorkspaceID)
	if err != nil {
		return ev, err
	}
	if cur == nil {
		ev.Skipped = append(ev.Skipped, "no alert configuration: no check runs")
		return ev, nil
	}
	ev.Evaluated = true
	doc := cur.Document
	now := a.clock.Now().UTC()

	// Freshness per account, for stale ingestion and for eligibility.
	fresh, err := a.reporting.ListFreshness(ctx, actor)
	if err != nil {
		return ev, err
	}
	accounts, err := a.db.Reader().ListAdAccounts(ctx)
	if err != nil {
		return ev, err
	}
	accountLabel := map[int64]string{}
	for _, acc := range accounts {
		accountLabel[acc.Account.ID] = acc.Account.Type + " " + acc.Account.Name.String
	}
	completeThrough := map[int64]reporting.Day{}
	for _, f := range fresh {
		if f.ObservedThrough != nil {
			completeThrough[f.AccountID] = *f.ObservedThrough
		}
		if doc.StaleIngestion.MaxAgeHours != nil {
			age := time.Duration(0)
			if f.LastSuccessAt != nil {
				age = now.Sub(*f.LastSuccessAt)
			}
			if f.LastSuccessAt == nil || age > time.Duration(*doc.StaleIngestion.MaxAgeHours)*time.Hour {
				msg := fmt.Sprintf("%s has no complete figures newer than %s; nothing has been read successfully", accountLabel[f.AccountID], hoursAgo(now, f.LastSuccessAt))
				if f.LastError != nil {
					msg += " (last error: " + *f.LastError + ")"
				}
				fired, err := a.raise(ctx, actor, cur.Version, CheckStaleIngestion, fmt.Sprintf("account:%d", f.AccountID), accountLabel[f.AccountID], "warning", msg, f)
				if err != nil {
					return ev, err
				}
				ev.count(fired)
			}
		}
	}
	if doc.StaleIngestion.MaxAgeHours == nil {
		ev.Skipped = append(ev.Skipped, "stale_ingestion: threshold not configured")
	}

	if doc.CostSpike.ThresholdPct == nil && doc.ZeroLeads.MinSpendMinor == nil {
		ev.Skipped = append(ev.Skipped, "cost_spike and zero_leads: thresholds not configured")
		return ev, nil
	}
	dayT, err := day.Time()
	if err != nil {
		return ev, core.Invalid("day must be YYYY-MM-DD")
	}
	today, err := a.reporting.Analytics(ctx, actor, reporting.AnalyticsQuery{From: day, To: day, GroupBy: "campaign"})
	if err != nil {
		return ev, err
	}
	baselineDays := doc.CostSpike.BaselineDays
	if baselineDays == 0 {
		baselineDays = 7
	}
	minBaseline := doc.CostSpike.MinBaselineDays
	if minBaseline == 0 {
		minBaseline = 3
	}
	// Baseline per campaign: total spend and the number of days it ran.
	type base struct {
		spend int64
		days  int
	}
	baseline := map[int64]base{}
	if doc.CostSpike.ThresholdPct != nil {
		rows, err := a.db.Reader().ListCampaignDailyMetrics(ctx, store.ListCampaignDailyMetricsParams{WorkspaceID: actor.WorkspaceID,
			FromDay: string(reporting.DayOf(dayT.AddDate(0, 0, -baselineDays))), ToDay: string(reporting.DayOf(dayT.AddDate(0, 0, -1)))})
		if err != nil {
			return ev, err
		}
		for _, r := range rows {
			b := baseline[r.CampaignID]
			b.spend += r.CostMinor
			b.days++
			baseline[r.CampaignID] = b
		}
	}
	campaignAccount := map[int64]int64{}
	campaigns, err := a.db.Reader().ListAllAdCampaigns(ctx, actor.WorkspaceID)
	if err != nil {
		return ev, err
	}
	for _, c := range campaigns {
		campaignAccount[c.ID] = c.AccountID
	}

	for _, row := range today.Rows {
		if row.Metrics.Spend.Minor == nil || row.CampaignID == nil {
			continue
		}
		spend := *row.Metrics.Spend.Minor
		accountID := campaignAccount[*row.CampaignID]
		// Eligibility: the day must be complete for the account. Otherwise
		// a zero is a gap, not an observation.
		complete := completeThrough[accountID] >= day
		if doc.CostSpike.ThresholdPct != nil {
			b := baseline[*row.CampaignID]
			switch {
			case doc.CostSpike.MinSpendMinor != nil && spend < *doc.CostSpike.MinSpendMinor:
			case b.days < minBaseline:
				ev.Skipped = append(ev.Skipped, fmt.Sprintf("cost_spike for %s: only %d baseline day(s), %d needed", row.Label, b.days, minBaseline))
			default:
				avg := float64(b.spend) / float64(b.days)
				if avg > 0 && float64(spend) > avg*(1+float64(*doc.CostSpike.ThresholdPct)/100) {
					pct := int((float64(spend)/avg - 1) * 100)
					msg := fmt.Sprintf("%s spent %s on %s, %d%% above its average of %s over the %d days it ran in the last %d. No change was made.", row.Label, fmtMoney(spend, row.Metrics.Spend.Currency), day, pct, fmtMoney(int64(avg), row.Metrics.Spend.Currency), b.days, baselineDays)
					fired, err := a.raise(ctx, actor, cur.Version, CheckCostSpike, row.Key, row.Label, "warning", msg, map[string]any{"day": day, "spendMinor": spend, "baselineAvgMinor": int64(avg), "baselineDays": b.days, "pctAbove": pct})
					if err != nil {
						return ev, err
					}
					ev.count(fired)
				}
			}
		}
		if doc.ZeroLeads.MinSpendMinor != nil && spend >= *doc.ZeroLeads.MinSpendMinor && row.Metrics.Leads == 0 {
			if !complete {
				ev.Skipped = append(ev.Skipped, fmt.Sprintf("zero_leads for %s: figures for %s are not complete yet", row.Label, day))
				continue
			}
			msg := fmt.Sprintf("%s spent %s on %s and no lead reached GoHighLevel. No change was made.", row.Label, fmtMoney(spend, row.Metrics.Spend.Currency), day)
			fired, err := a.raise(ctx, actor, cur.Version, CheckZeroLeads, row.Key, row.Label, "warning", msg, map[string]any{"day": day, "spendMinor": spend, "matchedLeads": today.Coverage.MatchedCount, "eligibleLeads": today.Coverage.EligibleCount})
			if err != nil {
				return ev, err
			}
			ev.count(fired)
		}
	}
	return ev, nil
}

func (e *AlertEvaluation) count(fresh bool) {
	if fresh {
		e.Fired++
	} else {
		e.Repeated++
	}
}

// Raise lets another domain record an evidence-backed observation as an
// alert: the same open-alert rules as the daily checks, under config version
// 0 because no alert config governs it. The check name must be stable.
func (a *Alerts) Raise(ctx context.Context, actor core.Actor, check, subjectKey, subjectLabel, severity, message string, evidence any) (bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return false, core.ErrForbidden
	}
	switch severity {
	case "info", "warning", "urgent":
	default:
		return false, core.Invalid("severity must be info, warning or urgent")
	}
	return a.raise(ctx, actor, 0, check, subjectKey, subjectLabel, severity, message, evidence)
}

// raise creates an alert or adds an occurrence to the open one for the same
// check and subject. It returns true when a new alert was created.
func (a *Alerts) raise(ctx context.Context, actor core.Actor, configVersion int64, check, subjectKey, subjectLabel, severity, message string, evidence any) (bool, error) {
	raw, _ := json.Marshal(evidence)
	now := a.clock.Now().UTC()
	created := false
	err := a.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		open, err := q.GetOpenAlert(ctx, store.GetOpenAlertParams{WorkspaceID: actor.WorkspaceID, CheckName: check, SubjectKey: subjectKey})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			_, err = q.CreateAlert(ctx, store.CreateAlertParams{WorkspaceID: actor.WorkspaceID, CheckName: check, SubjectKey: subjectKey, SubjectLabel: subjectLabel,
				Severity: severity, Message: message, Evidence: string(raw), ConfigVersion: configVersion, FirstSeenAt: now, LastSeenAt: now})
			created = err == nil
			return err
		case err != nil:
			return err
		}
		// Same day, same subject: the daily job re-running must not count twice.
		var last struct{ Day reporting.Day }
		_ = json.Unmarshal([]byte(open.Evidence), &last)
		var next struct{ Day reporting.Day }
		_ = json.Unmarshal(raw, &next)
		if last.Day != "" && last.Day == next.Day {
			return nil
		}
		_, err = q.TouchAlert(ctx, store.TouchAlertParams{LastSeenAt: now, Message: message, Evidence: string(raw), Severity: severity, ID: open.ID})
		return err
	})
	return created, err
}

func (a *Alerts) List(ctx context.Context, actor core.Actor, state string, limit int) ([]Alert, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := a.db.Reader().ListAlerts(ctx, store.ListAlertsParams{WorkspaceID: actor.WorkspaceID, State: nullString(state), Max: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Alert, 0, len(rows))
	for _, r := range rows {
		out = append(out, alertFromRow(r))
	}
	return out, nil
}

func (a *Alerts) Acknowledge(ctx context.Context, actor core.Actor, id int64) (Alert, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Alert{}, core.ErrForbidden
	}
	row, err := a.db.Writer().AcknowledgeAlert(ctx, store.AcknowledgeAlertParams{AcknowledgedBy: optionalID(actor.UserID), AcknowledgedAt: sql.NullTime{Time: a.clock.Now().UTC(), Valid: true}, ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Alert{}, core.NotFound("alert %d is not open", id)
	}
	if err != nil {
		return Alert{}, err
	}
	return alertFromRow(row), nil
}

func (a *Alerts) Resolve(ctx context.Context, actor core.Actor, id int64) (Alert, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Alert{}, core.ErrForbidden
	}
	row, err := a.db.Writer().ResolveAlert(ctx, store.ResolveAlertParams{ResolvedAt: sql.NullTime{Time: a.clock.Now().UTC(), Valid: true}, ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Alert{}, core.NotFound("alert %d is not open", id)
	}
	if err != nil {
		return Alert{}, err
	}
	return alertFromRow(row), nil
}

// EvaluateAll runs yesterday's checks for the default workspace as ADX. The
// daily job calls it.
func (a *Alerts) EvaluateAll(ctx context.Context) (AlertEvaluation, error) {
	yesterday := reporting.DayOf(a.clock.Now().In(reporting.ReportingZone).AddDate(0, 0, -1))
	return a.Evaluate(ctx, core.System(core.ViaJob), yesterday)
}

func unconfigured(cur *AlertConfig) []string {
	if cur == nil {
		return []string{"alert configuration"}
	}
	var out []string
	if cur.Document.CostSpike.ThresholdPct == nil {
		out = append(out, "cost_spike: threshold percent")
	}
	if cur.Document.ZeroLeads.MinSpendMinor == nil {
		out = append(out, "zero_leads: minimum spend")
	}
	if cur.Document.StaleIngestion.MaxAgeHours == nil {
		out = append(out, "stale_ingestion: maximum age")
	}
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func (a *Alerts) current(ctx context.Context, workspaceID int64) (*AlertConfig, error) {
	row, err := a.db.Reader().GetCurrentAlertConfig(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := configFromRow(row)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func configFromRow(r store.AlertConfig) (AlertConfig, error) {
	var doc AlertDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return AlertConfig{}, fmt.Errorf("alert config %d is not valid JSON: %w", r.ID, err)
	}
	out := AlertConfig{Version: r.Version, Document: doc, CreatedAt: r.CreatedAt.UTC()}
	if r.CreatedBy.Valid {
		out.CreatedBy = &r.CreatedBy.Int64
	}
	return out, nil
}

func alertFromRow(r store.Alert) Alert {
	out := Alert{ID: r.ID, Check: r.CheckName, SubjectKey: r.SubjectKey, SubjectLabel: r.SubjectLabel, Severity: r.Severity, State: r.State, Message: r.Message,
		Evidence: json.RawMessage(r.Evidence), ConfigVersion: r.ConfigVersion, Occurrences: r.Occurrences, FirstSeenAt: r.FirstSeenAt.UTC(), LastSeenAt: r.LastSeenAt.UTC()}
	if r.AcknowledgedBy.Valid {
		out.AcknowledgedBy = &r.AcknowledgedBy.Int64
	}
	if r.AcknowledgedAt.Valid {
		t := r.AcknowledgedAt.Time.UTC()
		out.AcknowledgedAt = &t
	}
	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time.UTC()
		out.ResolvedAt = &t
	}
	return out
}

func hoursAgo(now time.Time, t *time.Time) string {
	if t == nil {
		return "ever"
	}
	return fmt.Sprintf("%d hours ago", int(now.Sub(*t).Hours()))
}

func fmtMoney(minor int64, currency string) string {
	return fmt.Sprintf("%d.%02d %s", minor/100, minor%100, currency)
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
func optionalID(id int64) sql.NullInt64  { return sql.NullInt64{Int64: id, Valid: id != 0} }
