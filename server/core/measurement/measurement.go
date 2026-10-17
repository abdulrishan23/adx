// Package measurement says what ADX can count and whether a plan is ready
// to launch. It turns form submissions into events exactly once, with
// duplicates and the team's tests marked so they never count as enquiries;
// it runs controlled checks (a test submission, a conversion action that
// must exist, an account's permission) and keeps their evidence; and it
// joins the plan, its creatives, its pages, the accounts and the checks into
// one readiness verdict. What ADX cannot measure is listed as unsupported,
// never quietly left out. Nothing here is live: readiness says a plan may be
// put forward, and only an execution (B3.5) can confirm delivery.
package measurement

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

// MeasurementCatalog.

// MeasurementEventKind is something marketing might count.
type MeasurementEventKind struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Supported bool   `json:"supported"`
	// How is where the count comes from when supported, and why not when
	// it isn't.
	How string `json:"how"`
	// Platforms maps a platform to the artefact that should carry the
	// event there (a Google conversion action, a Meta pixel event), so the
	// plan's measurement requirements can be checked against reality.
	Platforms map[string]string `json:"platforms"`
}

var Events = []MeasurementEventKind{
	{Key: "form_submit", Label: "Form submission on an ADX page", Supported: true, How: "ADX receives the form and records one event per submission, server side.", Platforms: map[string]string{"google_ads": "conversion_action", "meta": "pixel_event"}},
	{Key: "whatsapp_click", Label: "Click on a WhatsApp button", Supported: false, How: "ADX pages carry no client script and no WhatsApp link tracking; needs a tracked redirect or a tag on the host site.", Platforms: map[string]string{}},
	{Key: "call_click", Label: "Click to call", Supported: false, How: "No client script; needs call tracking numbers or a tag on the host site.", Platforms: map[string]string{}},
	{Key: "page_view", Label: "Page view", Supported: false, How: "ADX pages carry no analytics script by policy (no third-party tags). Views are not counted.", Platforms: map[string]string{}},
	{Key: "crm_stage", Label: "CRM stage change (qualified, meeting, won)", Supported: true, How: "Read from GoHighLevel through the funnel mapping; counted in analytics, not as a page event.", Platforms: map[string]string{}},
}

// AttributionFields are what a submission may carry about where the click
// came from, and how each is matched later.
var AttributionFields = []string{"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term", "gclid", "gbraid", "wbraid", "fbclid", "referrer", "landing"}

// UTM is the scheme ADX builds links with. Fixed in code so every link reads
// the same way back in analytics.
type UTM struct {
	Source   string `json:"source"`   // google, meta
	Medium   string `json:"medium"`   // cpc, paid_social
	Campaign string `json:"campaign"` // plan-<id>-v<version>-<channel>
	Content  string `json:"content"`  // creative-<id>-v<version>
	Term     string `json:"term"`
}

// MeasurementCatalog is what the UI shows about measurement.
type MeasurementCatalog struct {
	Events            []MeasurementEventKind `json:"events"`
	AttributionFields []string               `json:"attributionFields"`
	UTMScheme         string                 `json:"utmScheme"`
	DedupeRule        string                 `json:"dedupeRule"`
}

func GetCatalog() MeasurementCatalog {
	return MeasurementCatalog{Events: Events, AttributionFields: AttributionFields,
		UTMScheme:  "utm_source=<google|meta>, utm_medium=<cpc|paid_social>, utm_campaign=plan-<plan>-v<version>-<channel>, utm_content=creative-<creative>-v<version>, utm_term optional",
		DedupeRule: "Two submissions with the same phone (last nine digits, so with or without the country code) or the same email, on the same page, within 24 hours are one enquiry; the later one is marked a duplicate and does not count."}
}

// TrackedURL appends the scheme to a page URL. Existing query parameters are
// kept; utm_* are replaced.
func TrackedURL(base string, u UTM) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", core.Invalid("a full page URL is required")
	}
	q := parsed.Query()
	set := func(k, v string) {
		if v == "" {
			q.Del(k)
			return
		}
		q.Set(k, v)
	}
	set("utm_source", u.Source)
	set("utm_medium", u.Medium)
	set("utm_campaign", u.Campaign)
	set("utm_content", u.Content)
	set("utm_term", u.Term)
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

// PlatformOf maps a plan channel to its platform and medium.
func PlatformOf(channel string) (source, medium string) {
	switch {
	case strings.HasPrefix(channel, "google_"):
		return "google", "cpc"
	case strings.HasPrefix(channel, "meta_"):
		return "meta", "paid_social"
	}
	return "", ""
}

// Events from submissions.

type MeasurementEvent struct {
	ID            int64             `json:"id"`
	Kind          string            `json:"kind"`
	Source        string            `json:"source"`
	SourceRef     string            `json:"sourceRef"`
	PageID        *int64            `json:"pageId"`
	ProjectID     *int64            `json:"projectId"`
	PlanID        *int64            `json:"planId"`
	Test          bool              `json:"test"`
	DedupeKey     string            `json:"dedupeKey"`
	DuplicateOfID *int64            `json:"duplicateOfId"`
	Counts        bool              `json:"counts"`
	Attribution   map[string]string `json:"attribution"`
	OccurredAt    time.Time         `json:"occurredAt"`
	RecordedAt    time.Time         `json:"recordedAt"`
}

type Config struct{ Clock core.Clock }

type Measurement struct {
	db          *store.DB
	clock       core.Clock
	evidence    *evidence.Store
	pages       *pages.Pages
	creative    *creative.Service
	plans       *plans.Plans
	connections *connections.Connections
}

func New(db *store.DB, ev *evidence.Store, pg *pages.Pages, cr *creative.Service, pl *plans.Plans, cn *connections.Connections, cfg Config) *Measurement {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Measurement{db: db, clock: cfg.Clock, evidence: ev, pages: pg, creative: cr, plans: pl, connections: cn}
}

var nonDigits = regexp.MustCompile(`[^0-9]`)

// dedupeKey is the person-on-this-page-today key: digits of the phone or
// the lowercased email, the page and the Dubai day. A submission with
// neither has no person to dedupe on and gets its own key.
func dedupeKey(pageID int64, fields map[string]string, at time.Time, fallback string) string {
	who := ""
	for k, v := range fields {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "phone") || strings.Contains(lk, "mobile") {
			// The last nine digits are the national number for UAE and most
			// GCC mobiles, so "+971 50 123 4567" and "050 123 4567" are one
			// person.
			if d := nonDigits.ReplaceAllString(v, ""); len(d) >= 7 {
				if len(d) > 9 {
					d = d[len(d)-9:]
				}
				who = "p:" + d
				break
			}
		}
	}
	if who == "" {
		for k, v := range fields {
			if strings.Contains(strings.ToLower(k), "email") && strings.Contains(v, "@") {
				who = "e:" + strings.ToLower(strings.TrimSpace(v))
				break
			}
		}
	}
	if who == "" {
		who = "s:" + fallback
	}
	day := at.In(reporting.ReportingZone).Format("2006-01-02")
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s", pageID, who, day)))
	return hex.EncodeToString(sum[:16])
}

// RecordPending turns every received form submission that has no event yet
// into exactly one event. Idempotent: the source reference is unique.
// Tests stay tests; a repeat of the same person on the same page within a
// day is a duplicate that does not count.
func (m *Measurement) RecordPending(ctx context.Context, actor core.Actor) (int, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	rows, err := m.db.Reader().ListUnrecordedSubmissions(ctx, actor.WorkspaceID)
	if err != nil {
		return 0, err
	}
	made := 0
	for _, s := range rows {
		if _, err := m.recordSubmission(ctx, actor, s); err != nil {
			return made, err
		}
		made++
	}
	return made, nil
}

func (m *Measurement) recordSubmission(ctx context.Context, actor core.Actor, s store.FormSubmission) (MeasurementEvent, error) {
	ref := fmt.Sprintf("form_submission:%d", s.ID)
	if existing, err := m.db.Reader().GetMeasurementEventBySourceRef(ctx, ref); err == nil {
		return eventFromRow(existing), nil
	}
	var fields map[string]string
	_ = json.Unmarshal([]byte(s.Fields), &fields)
	key := dedupeKey(s.PageID, fields, s.ReceivedAt, ref)
	var planID sql.NullInt64
	if p, err := m.db.Reader().GetPage(ctx, store.GetPageParams{ID: s.PageID, WorkspaceID: actor.WorkspaceID}); err == nil {
		planID = p.PlanID
	}
	var dup sql.NullInt64
	counts := s.Test == 0
	if earlier, err := m.db.Reader().FindMeasurementEventByDedupe(ctx, store.FindMeasurementEventByDedupeParams{WorkspaceID: actor.WorkspaceID, DedupeKey: key, OccurredAt: s.ReceivedAt.Add(-24 * time.Hour)}); err == nil && earlier.Test == s.Test {
		dup = sql.NullInt64{Int64: earlier.ID, Valid: true}
		counts = false
	}
	row, err := m.db.Writer().CreateMeasurementEvent(ctx, store.CreateMeasurementEventParams{WorkspaceID: actor.WorkspaceID, Kind: "form_submit", Source: "web_form", SourceRef: ref, PageID: sql.NullInt64{Int64: s.PageID, Valid: true}, ProjectID: s.ProjectID, PlanID: planID,
		Test: s.Test, DedupeKey: key, DuplicateOfID: dup, Counts: boolInt(counts), Attribution: s.Attribution, OccurredAt: s.ReceivedAt, RecordedAt: m.clock.Now().UTC()})
	if store.IsUniqueViolation(err) {
		existing, err := m.db.Reader().GetMeasurementEventBySourceRef(ctx, ref)
		if err != nil {
			return MeasurementEvent{}, err
		}
		return eventFromRow(existing), nil
	}
	if err != nil {
		return MeasurementEvent{}, err
	}
	return eventFromRow(row), nil
}

func (m *Measurement) ListEvents(ctx context.Context, actor core.Actor, pageID int64, limit int) ([]MeasurementEvent, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := m.db.Reader().ListMeasurementEvents(ctx, store.ListMeasurementEventsParams{WorkspaceID: actor.WorkspaceID, PageID: optionalID(pageID), Max: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]MeasurementEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, eventFromRow(r))
	}
	return out, nil
}

// CountEnquiries is how many events count as enquiries: not tests, not
// duplicates.
func (m *Measurement) CountEnquiries(ctx context.Context, actor core.Actor) (int64, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	return m.db.Reader().CountCountingEvents(ctx, actor.WorkspaceID)
}

// Checks.

type MeasurementCheckInput struct {
	PlanID      int64 `json:"planId"`
	PlanVersion int64 `json:"planVersion"`
	// Kind is form_submit, conversion_action, pixel_event or account_permission.
	Kind      string `json:"kind"`
	PageID    int64  `json:"pageId"`
	AccountID int64  `json:"accountId"`
	// Name is the conversion action or pixel event name for those kinds.
	Name string `json:"name"`
}

type MeasurementCheck struct {
	ID           int64     `json:"id"`
	PlanID       int64     `json:"planId"`
	PlanVersion  int64     `json:"planVersion"`
	Kind         string    `json:"kind"`
	PageID       *int64    `json:"pageId"`
	AccountID    *int64    `json:"accountId"`
	State        string    `json:"state"`
	Detail       string    `json:"detail"`
	SubmissionID *int64    `json:"submissionId"`
	EventID      *int64    `json:"eventId"`
	EvidenceID   *int64    `json:"evidenceId"`
	CheckedBy    *int64    `json:"checkedBy"`
	CheckedAt    time.Time `json:"checkedAt"`
}

// RunCheck runs one controlled verification and records it with evidence.
// Marketers may. A form_submit check posts a real submission through the
// page's own form path with the page's test token, so what is verified is
// the path a visitor takes; the submission is a test and never an enquiry.
func (m *Measurement) RunCheck(ctx context.Context, actor core.Actor, in MeasurementCheckInput) (MeasurementCheck, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return MeasurementCheck{}, core.ErrForbidden
	}
	pv, err := m.plans.Version(ctx, actor, in.PlanID, in.PlanVersion)
	if err != nil {
		return MeasurementCheck{}, err
	}
	res := store.CreateMeasurementCheckParams{WorkspaceID: actor.WorkspaceID, PlanID: in.PlanID, PlanVersion: pv.Version, Kind: in.Kind, PageID: optionalID(in.PageID), AccountID: optionalID(in.AccountID), CheckedBy: optionalID(actor.UserID), CheckedAt: m.clock.Now().UTC()}
	payload := map[string]any{"kind": in.Kind, "planId": in.PlanID, "planVersion": pv.Version}
	switch in.Kind {
	case "form_submit":
		state, detail, sub, ev := m.checkFormSubmit(ctx, actor, in.PageID)
		res.State, res.Detail = state, detail
		if sub != nil {
			res.SubmissionID = sql.NullInt64{Int64: sub.ID, Valid: true}
			payload["submissionId"] = sub.ID
		}
		if ev != nil {
			res.EventID = sql.NullInt64{Int64: ev.ID, Valid: true}
			payload["eventId"] = ev.ID
			payload["eventTest"] = ev.Test
			payload["eventCounts"] = ev.Counts
		}
	case "conversion_action":
		res.State, res.Detail = m.checkConversionAction(ctx, in.AccountID, in.Name)
	case "pixel_event":
		res.State = "unsupported"
		res.Detail = "ADX has no Meta Pixel or Conversions API integration, so it cannot verify that a pixel event fires; confirm it in Meta Events Manager and record the outcome here as a note"
	case "account_permission":
		res.State, res.Detail = m.checkAccountPermission(ctx, actor, in.AccountID)
	default:
		return MeasurementCheck{}, core.Invalid("kind must be form_submit, conversion_action, pixel_event or account_permission")
	}
	payload["state"], payload["detail"] = res.State, res.Detail
	if m.evidence != nil {
		ev, _, err := m.evidence.Record(ctx, actor, evidence.Observation{Source: "adx", Kind: "measurement_check", SourceRef: fmt.Sprintf("plan:%d/v%d/%s/%s", in.PlanID, pv.Version, in.Kind, m.clock.Now().UTC().Format(time.RFC3339Nano)), Scope: fmt.Sprintf("plan:%d", in.PlanID), Class: evidence.Observed, Payload: payload, Completeness: evidence.Complete})
		if err != nil {
			return MeasurementCheck{}, err
		}
		res.EvidenceID = sql.NullInt64{Int64: ev.ID, Valid: true}
	}
	row, err := m.db.Writer().CreateMeasurementCheck(ctx, res)
	if err != nil {
		return MeasurementCheck{}, err
	}
	return checkFromRow(row), nil
}

func (m *Measurement) checkFormSubmit(ctx context.Context, actor core.Actor, pageID int64) (string, string, *pages.Submission, *MeasurementEvent) {
	if pageID == 0 {
		return "failed", "a page is required for a form check", nil, nil
	}
	d, err := m.pages.Get(ctx, actor, pageID)
	if err != nil {
		return "failed", "page not found in this workspace", nil, nil
	}
	if d.Current == nil {
		return "failed", "the page is not published; publish it, then check", nil, nil
	}
	var live *pages.PageVersion
	for i := range d.Versions {
		if d.Versions[i].Version == d.Current.Version {
			live = &d.Versions[i]
		}
	}
	if live == nil || live.Document.Form == nil {
		return "failed", "the live version has no form", nil, nil
	}
	fields := map[string]string{}
	for _, f := range live.Document.Form.Fields {
		switch f.Kind {
		case "email":
			fields[f.Name] = "measurement-check@adx.invalid"
		case "phone":
			fields[f.Name] = "+971500000000"
		case "select":
			if len(f.Options) > 0 {
				fields[f.Name] = f.Options[0]
			}
		default:
			fields[f.Name] = "ADX measurement check"
		}
	}
	before, _ := m.db.Reader().CountCountingEvents(ctx, actor.WorkspaceID)
	sub, err := m.pages.Submit(ctx, pages.SubmissionInput{Slug: d.Page.Slug, Fields: fields, ProjectID: d.Page.ProjectID64(), VersionID: live.ID, TestToken: d.TestToken, RemoteIP: "adx-check", UserAgent: "ADX measurement check", Attribution: map[string]string{"utm_source": "adx_check"}})
	if err != nil {
		return "failed", "the test submission was rejected: " + err.Error(), nil, nil
	}
	if !sub.Test {
		return "failed", "the submission was not marked as a test; nothing further was recorded", &sub, nil
	}
	rows, err := m.db.Reader().ListUnrecordedSubmissions(ctx, actor.WorkspaceID)
	if err != nil {
		return "failed", "could not read submissions", &sub, nil
	}
	var ev *MeasurementEvent
	for _, r := range rows {
		e, err := m.recordSubmission(ctx, actor, r)
		if err != nil {
			return "failed", "could not record the event", &sub, nil
		}
		if r.ID == sub.ID {
			ev = &e
		}
	}
	if ev == nil {
		return "failed", "the test submission produced no event", &sub, nil
	}
	after, _ := m.db.Reader().CountCountingEvents(ctx, actor.WorkspaceID)
	if after != before {
		return "failed", "the test changed the enquiry count; a test must never count", &sub, ev
	}
	if !ev.Test || ev.Counts {
		return "failed", "the event is not marked as a non-counting test", &sub, ev
	}
	return "passed", fmt.Sprintf("one test submission on version %d became one test event (id %d) that does not count as an enquiry", live.Version, ev.ID), &sub, ev
}

func (m *Measurement) checkConversionAction(ctx context.Context, accountID int64, name string) (string, string) {
	if accountID == 0 || strings.TrimSpace(name) == "" {
		return "failed", "a Google Ads account and the conversion action's name are required"
	}
	acct, err := m.db.Reader().GetAccountByID(ctx, accountID)
	if err != nil {
		return "failed", "account not found"
	}
	if acct.Account.Type != "google_ads" {
		return "unsupported", "conversion actions are a Google Ads concept; for Meta use a pixel event"
	}
	rows, err := m.db.Reader().ListConversionActions(ctx, accountID)
	if err != nil {
		return "failed", "could not read conversion actions"
	}
	if len(rows) == 0 {
		return "failed", "no conversion actions have been read from this account yet; refresh it in Analytics, then check again"
	}
	for _, r := range rows {
		if strings.EqualFold(strings.TrimSpace(r.Name), strings.TrimSpace(name)) {
			status := strings.ToUpper(r.Status.String)
			if status != "" && status != "ENABLED" {
				return "failed", fmt.Sprintf("conversion action %q exists but its status is %s", r.Name, status)
			}
			if r.IncludeInConversions == 0 {
				return "failed", fmt.Sprintf("conversion action %q exists but is not included in Conversions; bids won't see it", r.Name)
			}
			return "passed", fmt.Sprintf("conversion action %q exists (%s) and is included in Conversions, as read on %s", r.Name, r.Type.String, r.LastSeenAt.UTC().Format("2006-01-02"))
		}
	}
	return "failed", fmt.Sprintf("no conversion action named %q in the account's last read (%d actions seen)", name, len(rows))
}

func (m *Measurement) checkAccountPermission(ctx context.Context, actor core.Actor, accountID int64) (string, string) {
	if m.connections == nil {
		return "failed", "connections are not configured on this server"
	}
	accounts, err := m.connections.ListAccounts(ctx, actor)
	if err != nil {
		return "failed", "could not list accounts"
	}
	for _, a := range accounts {
		if a.ID != accountID {
			continue
		}
		switch a.Health {
		case connections.HealthConnected:
			return "passed", fmt.Sprintf("%s account %s is connected and its read access was verified on %s; writes stay off until policy enables them (B3.4)", a.Provider, a.ExternalID, a.VerifiedAt.UTC().Format("2006-01-02"))
		case connections.HealthStale:
			return "failed", "the account's access was last verified more than a day ago; refresh it"
		default:
			return "failed", fmt.Sprintf("the account is %s", a.Health)
		}
	}
	return "failed", "account not found"
}

func (m *Measurement) ListChecks(ctx context.Context, actor core.Actor, planID, planVersion int64) ([]MeasurementCheck, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := m.db.Reader().ListMeasurementChecks(ctx, store.ListMeasurementChecksParams{WorkspaceID: actor.WorkspaceID, PlanID: planID, PlanVersion: planVersion})
	if err != nil {
		return nil, err
	}
	out := make([]MeasurementCheck, 0, len(rows))
	for _, r := range rows {
		out = append(out, checkFromRow(r))
	}
	return out, nil
}

// Launch readiness.

type LaunchItem struct {
	Area    string `json:"area"` // plan, assets, pages, accounts, measurement, external_review
	Code    string `json:"code"`
	State   string `json:"state"` // ok, blocked, warning, unsupported
	Message string `json:"message"`
	// Ref points at the thing: creative, page, account, check or plan id.
	RefKind string `json:"refKind"`
	RefID   int64  `json:"refId"`
}

type TrackedLink struct {
	CreativeID int64  `json:"creativeId"`
	PageID     int64  `json:"pageId"`
	Channel    string `json:"channel"`
	URL        string `json:"url"`
}

type PlanLaunchReadiness struct {
	PlanID      int64 `json:"planId"`
	PlanVersion int64 `json:"planVersion"`
	Ready       bool  `json:"ready"`
	// Live is always false here: readiness is not delivery.
	Live  bool          `json:"live"`
	Items []LaunchItem  `json:"items"`
	Links []TrackedLink `json:"links"`
	// ExecutionAuthorization is absent: nothing in measurement grants spend.
	ExecutionAuthorization *string `json:"executionAuthorization"`
}

// Readiness joins everything a launch needs and says what is missing. It
// never claims a thing is live.
func (m *Measurement) Readiness(ctx context.Context, actor core.Actor, planID, planVersion int64) (PlanLaunchReadiness, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return PlanLaunchReadiness{}, core.ErrForbidden
	}
	pv, err := m.plans.Version(ctx, actor, planID, planVersion)
	if err != nil {
		return PlanLaunchReadiness{}, err
	}
	r := PlanLaunchReadiness{PlanID: planID, PlanVersion: pv.Version, Items: []LaunchItem{}, Links: []TrackedLink{}}
	add := func(area, code, state, msg, refKind string, refID int64) {
		r.Items = append(r.Items, LaunchItem{Area: area, Code: code, State: state, Message: msg, RefKind: refKind, RefID: refID})
	}
	// Plan.
	if pv.State != plans.StateAccepted {
		add("plan", "plan_not_accepted", "blocked", fmt.Sprintf("plan version %d is %s; an admin must accept it", pv.Version, pv.State), "plan", planID)
	} else {
		add("plan", "plan_accepted", "ok", fmt.Sprintf("plan version %d accepted; acceptance authorizes no spend", pv.Version), "plan", planID)
	}
	// Assets: creatives bound to this plan version against its requirements.
	crows, err := m.db.Reader().ListCreativesByPlan(ctx, store.ListCreativesByPlanParams{WorkspaceID: actor.WorkspaceID, PlanID: sql.NullInt64{Int64: planID, Valid: true}})
	if err != nil {
		return PlanLaunchReadiness{}, err
	}
	readyByKey := map[string]int{}
	var readyCreatives []creativeRef
	for _, c := range crows {
		if c.PlanVersion.Valid && c.PlanVersion.Int64 != pv.Version {
			continue
		}
		rd, err := m.creative.Readiness(ctx, actor, c.ID)
		if err != nil {
			return PlanLaunchReadiness{}, err
		}
		if rd.Ready {
			readyByKey[c.Format+"|"+c.Language]++
			readyCreatives = append(readyCreatives, creativeRef{id: c.ID, version: rd.Version})
			add("assets", "creative_ready", "ok", fmt.Sprintf("%s (%s %s) version %d approved with approved claims", c.Name, c.Format, c.Language, rd.Version), "creative", c.ID)
			continue
		}
		for _, b := range rd.Blockers {
			add("assets", "creative_"+b.Code, "blocked", fmt.Sprintf("%s: %s", c.Name, b.Message), "creative", c.ID)
		}
	}
	for _, req := range pv.Document.AssetRequirements {
		key := string(req.Format) + "|" + string(req.Language)
		if readyByKey[key] < req.Count {
			add("assets", "assets_short", "blocked", fmt.Sprintf("the plan needs %d × %s %s; %d ready", req.Count, req.Format, req.Language, readyByKey[key]), "plan", planID)
		}
	}
	if len(pv.Document.AssetRequirements) == 0 && len(crows) == 0 {
		add("assets", "no_assets", "warning", "the plan has no asset requirements and no creatives are bound to it", "plan", planID)
	}
	// Pages.
	prows, err := m.db.Reader().ListPagesByPlan(ctx, store.ListPagesByPlanParams{WorkspaceID: actor.WorkspaceID, PlanID: sql.NullInt64{Int64: planID, Valid: true}})
	if err != nil {
		return PlanLaunchReadiness{}, err
	}
	var readyPages []pageRef
	for _, p := range prows {
		rd, err := m.pages.Readiness(ctx, actor, p.ID)
		if err != nil {
			return PlanLaunchReadiness{}, err
		}
		if rd.Ready {
			readyPages = append(readyPages, pageRef{id: p.ID, slug: p.Slug})
			add("pages", "page_ready", "ok", fmt.Sprintf("/p/%s live (version %d), form tested, claims approved", p.Slug, *rd.PublishedVersion), "page", p.ID)
			continue
		}
		for _, b := range rd.Blockers {
			add("pages", "page_"+b.Code, "blocked", fmt.Sprintf("/p/%s: %s", p.Slug, b.Message), "page", p.ID)
		}
	}
	if len(pv.Document.PageRequirements) > 0 && len(readyPages) == 0 {
		add("pages", "no_ready_page", "blocked", "the plan requires a landing page and none bound to it is ready", "plan", planID)
	}
	// Accounts: one connected account per platform the plan spends on.
	platforms := map[string]bool{}
	for _, a := range pv.Document.Allocations {
		if src, _ := PlatformOf(a.Channel); src != "" {
			platforms[src] = true
		} else {
			add("accounts", "channel_manual", "unsupported", fmt.Sprintf("%s is planned but ADX cannot deploy or measure it; it stays manual", a.Channel), "plan", planID)
		}
	}
	var accounts []connections.Account
	if m.connections != nil {
		accounts, _ = m.connections.ListAccounts(ctx, actor)
	}
	for _, src := range []string{"google", "meta"} {
		if !platforms[src] {
			continue
		}
		provider := map[string]string{"google": "google_ads", "meta": "meta"}[src]
		found := false
		for _, a := range accounts {
			if a.Provider != provider {
				continue
			}
			found = true
			switch a.Health {
			case connections.HealthConnected:
				add("accounts", "account_connected", "ok", fmt.Sprintf("%s account %s connected (read); writes need policy (B3.4)", provider, a.ExternalID), "account", a.ID)
			default:
				add("accounts", "account_"+string(a.Health), "blocked", fmt.Sprintf("%s account %s is %s", provider, a.ExternalID, a.Health), "account", a.ID)
			}
		}
		if !found {
			add("accounts", "account_missing", "blocked", fmt.Sprintf("no %s account is connected", provider), "plan", planID)
		}
	}
	// Measurement: every requirement needs a passed check on this version.
	checks, err := m.ListChecks(ctx, actor, planID, pv.Version)
	if err != nil {
		return PlanLaunchReadiness{}, err
	}
	latestByKind := map[string]MeasurementCheck{}
	for i := len(checks) - 1; i >= 0; i-- {
		latestByKind[checks[i].Kind] = checks[i]
	}
	for _, req := range pv.Document.MeasurementRequirements {
		switch req.Kind {
		case "conversion_action", "pixel_event":
			c, ok := latestByKind[req.Kind]
			switch {
			case !ok && req.Kind == "pixel_event":
				add("measurement", "pixel_unverifiable", "unsupported", fmt.Sprintf("%s %q: ADX cannot verify Meta pixel events; confirm in Events Manager", req.Kind, req.Name), "plan", planID)
			case !ok:
				add("measurement", "check_missing", "blocked", fmt.Sprintf("%s %q has not been checked on this version", req.Kind, req.Name), "plan", planID)
			case c.State == "passed":
				add("measurement", "check_passed", "ok", fmt.Sprintf("%s %q: %s", req.Kind, req.Name, c.Detail), "check", c.ID)
			case c.State == "unsupported":
				add("measurement", "check_unsupported", "unsupported", fmt.Sprintf("%s %q: %s", req.Kind, req.Name, c.Detail), "check", c.ID)
			default:
				add("measurement", "check_failed", "blocked", fmt.Sprintf("%s %q: %s", req.Kind, req.Name, c.Detail), "check", c.ID)
			}
		case "utm_scheme":
			add("measurement", "utm_scheme", "ok", "links are built with the fixed scheme: "+GetCatalog().UTMScheme, "plan", planID)
		case "crm_stage":
			add("measurement", "crm_stage", "ok", "stages are read from the CRM through the funnel mapping", "plan", planID)
		case "call_tracking":
			add("measurement", "call_tracking_unsupported", "unsupported", "ADX has no call tracking; counted only if the team records it", "plan", planID)
		}
	}
	if len(readyPages) > 0 {
		c, ok := latestByKind["form_submit"]
		switch {
		case !ok:
			add("measurement", "form_check_missing", "blocked", "no controlled form check has been run on this plan version", "plan", planID)
		case c.State == "passed":
			add("measurement", "form_check_passed", "ok", c.Detail, "check", c.ID)
		default:
			add("measurement", "form_check_failed", "blocked", c.Detail, "check", c.ID)
		}
	}
	// External review: nothing has been submitted anywhere, so nothing is
	// under review and nothing is live.
	add("external_review", "not_submitted", "warning", "no ad has been submitted to a platform; platform review happens after an authorized execution (B3.5), and until it passes nothing is live", "plan", planID)
	// Tracked links for every ready creative on every ready page.
	for _, pg := range readyPages {
		for _, cr := range readyCreatives {
			for _, a := range pv.Document.Allocations {
				src, medium := PlatformOf(a.Channel)
				if src == "" {
					continue
				}
				u, err := TrackedURL(m.pages.PublicURL(pg.slug), UTM{Source: src, Medium: medium, Campaign: fmt.Sprintf("plan-%d-v%d-%s", planID, pv.Version, a.Channel), Content: fmt.Sprintf("creative-%d-v%d", cr.id, cr.version)})
				if err == nil {
					r.Links = append(r.Links, TrackedLink{CreativeID: cr.id, PageID: pg.id, Channel: a.Channel, URL: u})
				}
			}
		}
	}
	r.Ready = true
	for _, it := range r.Items {
		if it.State == "blocked" {
			r.Ready = false
		}
	}
	return r, nil
}

type creativeRef struct{ id, version int64 }
type pageRef struct {
	id   int64
	slug string
}

func eventFromRow(r store.MeasurementEvent) MeasurementEvent {
	e := MeasurementEvent{ID: r.ID, Kind: r.Kind, Source: r.Source, SourceRef: r.SourceRef, PageID: idPtr(r.PageID), ProjectID: idPtr(r.ProjectID), PlanID: idPtr(r.PlanID), Test: r.Test == 1, DedupeKey: r.DedupeKey, DuplicateOfID: idPtr(r.DuplicateOfID), Counts: r.Counts == 1, Attribution: map[string]string{}, OccurredAt: r.OccurredAt.UTC(), RecordedAt: r.RecordedAt.UTC()}
	_ = json.Unmarshal([]byte(r.Attribution), &e.Attribution)
	return e
}

func checkFromRow(r store.MeasurementCheck) MeasurementCheck {
	return MeasurementCheck{ID: r.ID, PlanID: r.PlanID, PlanVersion: r.PlanVersion, Kind: r.Kind, PageID: idPtr(r.PageID), AccountID: idPtr(r.AccountID), State: r.State, Detail: r.Detail, SubmissionID: idPtr(r.SubmissionID), EventID: idPtr(r.EventID), EvidenceID: idPtr(r.EvidenceID), CheckedBy: idPtr(r.CheckedBy), CheckedAt: r.CheckedAt.UTC()}
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

var _ = errors.Is
