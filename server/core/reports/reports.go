// Package reports builds and keeps scheduled and ad hoc reports. A report is
// one occurrence of a schedule for one period: the reporting result at the
// time, frozen with its formula version and freshness, so what was delivered
// can be shown again exactly. Occurrence keys stop a repeated schedule tick
// from building or sending the same report twice. Delivery needs
// recipients (the owner's input) and a mail transport; without either the
// report is still generated and shown, with delivery marked unconfigured.
package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

type Cadence string

const (
	Daily     Cadence = "daily"
	Weekly    Cadence = "weekly"
	Monthly   Cadence = "monthly"
	Quarterly Cadence = "quarterly"
)

func (c Cadence) valid() bool {
	switch c {
	case Daily, Weekly, Monthly, Quarterly:
		return true
	}
	return false
}

// ReportParams is the report's scope: what the analytics query gets.
type ReportParams struct {
	AccountID int64  `json:"accountId"`
	Platform  string `json:"platform"`
	ProjectID int64  `json:"projectId"`
	GroupBy   string `json:"groupBy"`
	// Compare is previous or year, or empty.
	Compare string `json:"compare"`
}

type ReportSchedule struct {
	ID         int64        `json:"id"`
	Name       string       `json:"name"`
	Cadence    Cadence      `json:"cadence"`
	Enabled    bool         `json:"enabled"`
	Params     ReportParams `json:"params"`
	Recipients []string     `json:"recipients"`
	// DeliveryConfigured is false while recipients are empty or no mail
	// transport exists: the report will be generated, not sent.
	DeliveryConfigured bool      `json:"deliveryConfigured"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type ReportScheduleInput struct {
	Name       string       `json:"name"`
	Cadence    Cadence      `json:"cadence"`
	Enabled    bool         `json:"enabled"`
	Params     ReportParams `json:"params"`
	Recipients []string     `json:"recipients"`
}

type Report struct {
	ID             int64                      `json:"id"`
	ScheduleID     *int64                     `json:"scheduleId"`
	OccurrenceKey  string                     `json:"occurrenceKey"`
	Cadence        Cadence                    `json:"cadence"`
	PeriodStart    reporting.Day              `json:"periodStart"`
	PeriodEnd      reporting.Day              `json:"periodEnd"`
	Params         ReportParams               `json:"params"`
	FormulaVersion string                     `json:"formulaVersion"`
	Result         *reporting.ReportingResult `json:"result"`
	Status         string                     `json:"status"`
	Error          *string                    `json:"error"`
	DeliveryStatus string                     `json:"deliveryStatus"`
	DeliveryDetail *string                    `json:"deliveryDetail"`
	DeliveredAt    *time.Time                 `json:"deliveredAt"`
	GeneratedAt    time.Time                  `json:"generatedAt"`
}

// Message is what the mail transport sends. Body is plain text built from
// the deterministic result; no model is involved.
type Message struct {
	To      []string
	Subject string
	Body    string
}

// Mailer is the outgoing mail transport, injected by cmd/adx when SMTP is
// configured. Nil means unconfigured.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

type Config struct{ Clock core.Clock }

type Reports struct {
	db        *store.DB
	reporting *reporting.Reporting
	mailer    Mailer
	clock     core.Clock
}

func New(db *store.DB, r *reporting.Reporting, mailer Mailer, cfg Config) *Reports {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Reports{db: db, reporting: r, mailer: mailer, clock: cfg.Clock}
}

func (r *Reports) ListSchedules(ctx context.Context, actor core.Actor) ([]ReportSchedule, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := r.db.Reader().ListReportSchedules(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]ReportSchedule, 0, len(rows))
	for _, row := range rows {
		out = append(out, r.scheduleFromRow(row))
	}
	return out, nil
}

// CreateSchedule adds a schedule. Admins only. Recipients may be empty,
// which leaves delivery unconfigured on purpose.
func (r *Reports) CreateSchedule(ctx context.Context, actor core.Actor, in ReportScheduleInput) (ReportSchedule, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return ReportSchedule{}, core.ErrForbidden
	}
	params, recipients, err := validateSchedule(in)
	if err != nil {
		return ReportSchedule{}, err
	}
	now := r.clock.Now().UTC()
	row, err := r.db.Writer().CreateReportSchedule(ctx, store.CreateReportScheduleParams{
		WorkspaceID: actor.WorkspaceID, Name: strings.TrimSpace(in.Name), Cadence: string(in.Cadence), Enabled: boolInt(in.Enabled),
		Params: params, Recipients: recipients, CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return ReportSchedule{}, err
	}
	return r.scheduleFromRow(row), nil
}

func (r *Reports) UpdateSchedule(ctx context.Context, actor core.Actor, id int64, in ReportScheduleInput) (ReportSchedule, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return ReportSchedule{}, core.ErrForbidden
	}
	params, recipients, err := validateSchedule(in)
	if err != nil {
		return ReportSchedule{}, err
	}
	row, err := r.db.Writer().UpdateReportSchedule(ctx, store.UpdateReportScheduleParams{
		Name: strings.TrimSpace(in.Name), Cadence: string(in.Cadence), Enabled: boolInt(in.Enabled), Params: params, Recipients: recipients,
		UpdatedAt: r.clock.Now().UTC(), ID: id, WorkspaceID: actor.WorkspaceID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return ReportSchedule{}, core.NotFound("schedule %d not found", id)
	}
	if err != nil {
		return ReportSchedule{}, err
	}
	return r.scheduleFromRow(row), nil
}

func validateSchedule(in ReportScheduleInput) (string, string, error) {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 100 {
		return "", "", core.Invalid("a schedule needs a name of up to 100 characters")
	}
	if !in.Cadence.valid() {
		return "", "", core.Invalid("cadence must be daily, weekly, monthly or quarterly")
	}
	switch in.Params.GroupBy {
	case "", "campaign", "project", "platform", "day":
	default:
		return "", "", core.Invalid("groupBy must be campaign, project, platform or day")
	}
	switch in.Params.Compare {
	case "", "previous", "year":
	default:
		return "", "", core.Invalid("compare must be previous, year or empty")
	}
	recipients := make([]string, 0, len(in.Recipients))
	for _, rcp := range in.Recipients {
		addr := strings.ToLower(strings.TrimSpace(rcp))
		if a, err := mail.ParseAddress(addr); err != nil || a.Address != addr {
			return "", "", core.Invalid("%q is not a valid email address", rcp)
		}
		recipients = append(recipients, addr)
	}
	p, _ := json.Marshal(in.Params)
	rcp, _ := json.Marshal(recipients)
	return string(p), string(rcp), nil
}

// Period is the completed period a cadence covers as of now, in Dubai time:
// yesterday, last Monday to Sunday, last month, last quarter.
func Period(cadence Cadence, now time.Time) (reporting.Day, reporting.Day) {
	n := now.In(reporting.ReportingZone)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, reporting.ReportingZone)
	switch cadence {
	case Weekly:
		// Last full week, Monday to Sunday.
		weekday := int(today.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		thisMonday := today.AddDate(0, 0, -(weekday - 1))
		return reporting.DayOf(thisMonday.AddDate(0, 0, -7)), reporting.DayOf(thisMonday.AddDate(0, 0, -1))
	case Monthly:
		first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, reporting.ReportingZone)
		return reporting.DayOf(first.AddDate(0, -1, 0)), reporting.DayOf(first.AddDate(0, 0, -1))
	case Quarterly:
		q := (int(today.Month()) - 1) / 3
		first := time.Date(today.Year(), time.Month(q*3+1), 1, 0, 0, 0, 0, reporting.ReportingZone)
		return reporting.DayOf(first.AddDate(0, -3, 0)), reporting.DayOf(first.AddDate(0, 0, -1))
	}
	y := today.AddDate(0, 0, -1)
	return reporting.DayOf(y), reporting.DayOf(y)
}

// Generate builds the report a schedule is due for now. The occurrence is
// the schedule and the period start, so ticking twice reuses the report
// (created=false) and delivers nothing again.
func (r *Reports) Generate(ctx context.Context, actor core.Actor, scheduleID int64) (Report, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Report{}, false, core.ErrForbidden
	}
	row, err := r.db.Reader().GetReportSchedule(ctx, store.GetReportScheduleParams{ID: scheduleID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, false, core.NotFound("schedule %d not found", scheduleID)
	}
	if err != nil {
		return Report{}, false, err
	}
	sched := r.scheduleFromRow(row)
	from, to := Period(sched.Cadence, r.clock.Now())
	key := fmt.Sprintf("schedule:%d/%s", sched.ID, from)
	return r.generate(ctx, actor, &sched, key, from, to, sched.Params, sched.Recipients)
}

// GenerateAdHoc builds a one-off report for a period. Marketers may. The
// occurrence is the period and params, so asking twice for the same thing
// shows the same report.
func (r *Reports) GenerateAdHoc(ctx context.Context, actor core.Actor, from, to reporting.Day, params ReportParams) (Report, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Report{}, false, core.ErrForbidden
	}
	p, _ := json.Marshal(params)
	key := fmt.Sprintf("adhoc/%s..%s/%s", from, to, p)
	return r.generate(ctx, actor, nil, key, from, to, params, nil)
}

func (r *Reports) generate(ctx context.Context, actor core.Actor, sched *ReportSchedule, key string, from, to reporting.Day, params ReportParams, recipients []string) (Report, bool, error) {
	if existing, err := r.db.Reader().GetReportByOccurrence(ctx, store.GetReportByOccurrenceParams{WorkspaceID: actor.WorkspaceID, OccurrenceKey: key}); err == nil {
		return r.reportFromRow(existing), false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Report{}, false, err
	}
	now := r.clock.Now().UTC()
	res, err := r.reporting.Analytics(ctx, actor, reporting.AnalyticsQuery{From: from, To: to, AccountID: params.AccountID, Platform: params.Platform,
		ProjectID: params.ProjectID, GroupBy: params.GroupBy, Compare: params.Compare})
	paramsJSON, _ := json.Marshal(params)
	create := store.CreateReportParams{
		WorkspaceID: actor.WorkspaceID, OccurrenceKey: key, Cadence: "adhoc", PeriodStart: string(from), PeriodEnd: string(to), Params: string(paramsJSON),
		FormulaVersion: reporting.FormulaVersion, Result: "{}", Status: "generated", DeliveryStatus: "unconfigured", GeneratedAt: now, RequestedBy: optionalID(actor.UserID),
	}
	if sched != nil {
		create.ScheduleID = sql.NullInt64{Int64: sched.ID, Valid: true}
		create.Cadence = string(sched.Cadence)
	}
	if err != nil {
		var ce *core.Error
		if !errors.As(err, &ce) {
			return Report{}, false, err
		}
		create.Status = "failed"
		create.ErrorSafe = sql.NullString{String: ce.Message, Valid: true}
	} else {
		raw, _ := json.Marshal(res)
		create.Result = string(raw)
		switch {
		case len(recipients) == 0:
			create.DeliveryDetail = sql.NullString{String: "no recipients configured", Valid: true}
		case r.mailer == nil:
			create.DeliveryDetail = sql.NullString{String: "no mail transport configured", Valid: true}
		default:
			create.DeliveryStatus = "pending"
		}
	}
	row, err := r.db.Writer().CreateReport(ctx, create)
	if store.IsUniqueViolation(err) {
		existing, err := r.db.Reader().GetReportByOccurrence(ctx, store.GetReportByOccurrenceParams{WorkspaceID: actor.WorkspaceID, OccurrenceKey: key})
		if err != nil {
			return Report{}, false, err
		}
		return r.reportFromRow(existing), false, nil
	}
	if err != nil {
		return Report{}, false, err
	}
	report := r.reportFromRow(row)
	if row.DeliveryStatus == "pending" {
		report = r.deliver(ctx, report, recipients, sched)
	}
	return report, true, nil
}

// deliver sends the report once. The pending → sent/failed update is
// conditional on still being pending, so two deliverers can't both send.
func (r *Reports) deliver(ctx context.Context, report Report, recipients []string, sched *ReportSchedule) Report {
	subject := fmt.Sprintf("ADX %s report: %s to %s", report.Cadence, report.PeriodStart, report.PeriodEnd)
	if sched != nil {
		subject = fmt.Sprintf("ADX: %s (%s to %s)", sched.Name, report.PeriodStart, report.PeriodEnd)
	}
	body := renderText(report)
	now := r.clock.Now().UTC()
	status, detail := "sent", ""
	if err := r.mailer.Send(ctx, Message{To: recipients, Subject: subject, Body: body}); err != nil {
		status, detail = "failed", safeMessage(err)
	}
	n, err := r.db.Writer().SetReportDelivery(ctx, store.SetReportDeliveryParams{
		DeliveryStatus: status, DeliveryDetail: nullString(detail), DeliveredAt: sql.NullTime{Time: now, Valid: status == "sent"}, ID: report.ID,
	})
	if err == nil && n == 1 {
		report.DeliveryStatus = status
		if detail != "" {
			report.DeliveryDetail = &detail
		}
		if status == "sent" {
			report.DeliveredAt = &now
		}
	}
	return report
}

// renderText writes the report as plain text from the stored result. Every
// number is the engine's; missing figures say so.
func renderText(rep Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Period %s to %s (%s). Formula %s.\n", rep.PeriodStart, rep.PeriodEnd, reporting.ReportingZone, rep.FormulaVersion)
	if rep.Result == nil {
		b.WriteString("No figures.\n")
		return b.String()
	}
	res := rep.Result
	if res.Freshness.ObservedThrough != nil {
		fmt.Fprintf(&b, "Figures observed through %s", *res.Freshness.ObservedThrough)
		if res.Freshness.Partial {
			b.WriteString(" (partial)")
		}
		b.WriteString(".\n")
	} else {
		b.WriteString("Figures: never read.\n")
	}
	fmt.Fprintf(&b, "Lead coverage: %d of %d matched to a campaign, %d with a status.\n", res.Coverage.MatchedCount, res.Coverage.EligibleCount, res.Coverage.WithStatus)
	for _, d := range res.Coverage.DependenciesMissing {
		fmt.Fprintf(&b, "Not available: %s\n", d)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(&b, "Warning: %s\n", w.Message)
	}
	b.WriteString("\n")
	write := func(label string, m reporting.ReportMetrics) {
		fmt.Fprintf(&b, "%s: spend %s, leads %d, qualified %s, deals %s, cost/lead %s, cost/qualified %s, Google conv. %s, Meta leads %s, revenue %s, ROAS %s\n",
			label, money(m.Spend), m.Leads, count(m.QualifiedLeads), count(m.Deals), ratio(m.CostPerLead, m.Spend.Currency), ratio(m.CostPerQualifiedLead, m.Spend.Currency),
			fl(m.GoogleConversions), fl(m.MetaLeads), money(m.Revenue), ratioPlain(m.ROAS))
	}
	for _, row := range res.Rows {
		write(row.Label, row.Metrics)
	}
	write("Total", res.Summary.Metrics)
	return b.String()
}

func money(m reporting.Money) string {
	if m.Minor == nil {
		return "not available"
	}
	return fmt.Sprintf("%d.%02d %s", *m.Minor/100, abs(*m.Minor%100), m.Currency)
}
func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
func count(v *int64) string {
	if v == nil {
		return "not available"
	}
	return fmt.Sprintf("%d", *v)
}
func fl(v *float64) string {
	if v == nil {
		return "not available"
	}
	return fmt.Sprintf("%.1f", *v)
}
func ratio(r reporting.Ratio, currency string) string {
	if r.Undefined {
		return "no leads"
	}
	if r.Value == nil {
		return "not available"
	}
	minor := int64(*r.Value)
	return money(reporting.Money{Minor: &minor, Currency: currency})
}
func ratioPlain(r reporting.Ratio) string {
	if r.Value == nil {
		return "not available"
	}
	return fmt.Sprintf("%.2f", *r.Value)
}

func (r *Reports) Get(ctx context.Context, actor core.Actor, id int64) (Report, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Report{}, core.ErrForbidden
	}
	row, err := r.db.Reader().GetReport(ctx, store.GetReportParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, core.NotFound("report %d not found", id)
	}
	if err != nil {
		return Report{}, err
	}
	return r.reportFromRow(row), nil
}

func (r *Reports) List(ctx context.Context, actor core.Actor, limit int) ([]Report, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Reader().ListReports(ctx, store.ListReportsParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Report, 0, len(rows))
	for _, row := range rows {
		rep := r.reportFromRow(row)
		rep.Result = nil // the list stays light; Get returns the snapshot
		out = append(out, rep)
	}
	return out, nil
}

// RunDue generates every enabled schedule's current period, across
// workspaces, as ADX itself. The periodic job calls it once a day; the
// occurrence keys make a second call within the same period a no-op.
func (r *Reports) RunDue(ctx context.Context) (generated int, errs []error) {
	rows, err := r.db.Reader().ListEnabledSchedulesAllWorkspaces(ctx)
	if err != nil {
		return 0, []error{err}
	}
	for _, row := range rows {
		actor := core.SystemIn(core.ViaJob, row.WorkspaceID)
		_, created, err := r.Generate(ctx, actor, row.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("schedule %d: %w", row.ID, err))
			continue
		}
		if created {
			generated++
		}
	}
	return generated, errs
}

func (r *Reports) scheduleFromRow(row store.ReportSchedule) ReportSchedule {
	var params ReportParams
	_ = json.Unmarshal([]byte(row.Params), &params)
	recipients := []string{}
	_ = json.Unmarshal([]byte(row.Recipients), &recipients)
	return ReportSchedule{ID: row.ID, Name: row.Name, Cadence: Cadence(row.Cadence), Enabled: row.Enabled == 1, Params: params, Recipients: recipients,
		DeliveryConfigured: len(recipients) > 0 && r.mailer != nil, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC()}
}

func (r *Reports) reportFromRow(row store.Report) Report {
	var params ReportParams
	_ = json.Unmarshal([]byte(row.Params), &params)
	out := Report{ID: row.ID, OccurrenceKey: row.OccurrenceKey, Cadence: Cadence(row.Cadence), PeriodStart: reporting.Day(row.PeriodStart), PeriodEnd: reporting.Day(row.PeriodEnd),
		Params: params, FormulaVersion: row.FormulaVersion, Status: row.Status, DeliveryStatus: row.DeliveryStatus, GeneratedAt: row.GeneratedAt.UTC()}
	if row.ScheduleID.Valid {
		out.ScheduleID = &row.ScheduleID.Int64
	}
	if row.ErrorSafe.Valid {
		out.Error = &row.ErrorSafe.String
	}
	if row.DeliveryDetail.Valid {
		out.DeliveryDetail = &row.DeliveryDetail.String
	}
	if row.DeliveredAt.Valid {
		t := row.DeliveredAt.Time.UTC()
		out.DeliveredAt = &t
	}
	if row.Status == "generated" && row.Result != "{}" {
		var res reporting.ReportingResult
		if json.Unmarshal([]byte(row.Result), &res) == nil {
			out.Result = &res
		}
	}
	return out
}

func safeMessage(err error) string {
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Message
	}
	return "delivery failed; see the server log"
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
func optionalID(id int64) sql.NullInt64  { return sql.NullInt64{Int64: id, Valid: id != 0} }
func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
