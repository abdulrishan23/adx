package leads

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
)

// Contact is a CRM contact as the provider adapter returns it, already
// decoded but otherwise verbatim. Raw is the provider's JSON for evidence.
type Contact struct {
	ID           string
	FirstName    string
	LastName     string
	Email        string
	Phone        string
	Source       string
	Country      string
	DateAdded    *time.Time
	DateUpdated  *time.Time
	Tags         []string
	CustomFields map[string]string
	Attributions []Attribution
	Raw          json.RawMessage
}

// Attribution is one of the CRM's attribution records for a contact.
// GoHighLevel keeps two: the first touch (IsFirst) and the last touch.
type Attribution struct {
	IsFirst     bool
	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	UTMContent  string
	UTMTerm     string
	CampaignID  string
	AdID        string
	GCLID       string
	GBRAID      string
	WBRAID      string
	FBCLID      string
	Referrer    string
	LandingURL  string
}

// ContactsPage is one page of contacts. NextCursor is empty on the last page.
// Total is the provider's count when it gives one, so the run can tell
// complete from partial.
type ContactsPage struct {
	Contacts   []Contact
	NextCursor string
	Total      *int64
}

// CRM is the read side of GoHighLevel core needs, implemented by
// providers/gohighlevel and injected by cmd/adx. updatedSince narrows an
// incremental import; nil reads everything.
type CRM interface {
	ListContacts(ctx context.Context, token, locationID, cursor string, updatedSince *time.Time) (ContactsPage, error)
}

// Tokens hands out a location's access token for a read. core/connections
// implements it; the token never leaves core.
type Tokens interface {
	GHLToken(ctx context.Context, actor core.Actor, locationID string) (string, error)
}

// Lead is a lead as the API shows it.
type Lead struct {
	ID                 int64      `json:"id"`
	AccountID          int64      `json:"accountId"`
	ExternalID         string     `json:"externalId"`
	FirstName          *string    `json:"firstName"`
	LastName           *string    `json:"lastName"`
	Email              *string    `json:"email"`
	Phone              *string    `json:"phone"`
	DialCode           *string    `json:"dialCode"`
	SourceRaw          *string    `json:"sourceRaw"`
	Source             *string    `json:"source"`
	ProjectRaw         *string    `json:"projectRaw"`
	ProjectID          *int64     `json:"projectId"`
	StatusRaw          *string    `json:"statusRaw"`
	StatusObservedAt   *time.Time `json:"statusObservedAt"`
	StatusMissingSince *time.Time `json:"statusMissingSince"`
	Stage              *Stage     `json:"stage"`
	ProviderAddedAt    *time.Time `json:"providerAddedAt"`
	ProviderUpdatedAt  *time.Time `json:"providerUpdatedAt"`
	FirstSeenAt        time.Time  `json:"firstSeenAt"`
	LastSeenAt         time.Time  `json:"lastSeenAt"`
	EvidenceID         *int64     `json:"evidenceId"`
}

// Coverage is the data-quality view: the share of leads that have what the
// figures depend on (reference.md).
type Coverage struct {
	Total         int64            `json:"total"`
	WithStatus    int64            `json:"withStatus"`
	WithStage     int64            `json:"withStage"`
	WithProject   int64            `json:"withProject"`
	WithPhone     int64            `json:"withPhone"`
	StatusMissing int64            `json:"statusMissingOnLastScan"`
	SharedPhones  int64            `json:"sharedPhones"`
	MatchLevels   map[string]int64 `json:"matchLevels"`
	// FunnelConfigured says whether stages mean anything yet.
	FunnelConfigured bool `json:"funnelConfigured"`
}

type Config struct {
	Clock core.Clock
}

type Leads struct {
	db       *store.DB
	crm      CRM
	tokens   Tokens
	evidence *evidence.Store
	clock    core.Clock
}

// New wires the leads service. crm and tokens may be nil when no GoHighLevel
// is configured; imports then fail with a clear message and reads still work.
func New(db *store.DB, crm CRM, tokens Tokens, ev *evidence.Store, cfg Config) *Leads {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Leads{db: db, crm: crm, tokens: tokens, evidence: ev, clock: cfg.Clock}
}

// ListFilter narrows List. AfterID pages by id.
type ListFilter struct {
	AccountID int64
	Stage     string
	StatusRaw string
	AfterID   int64
	Limit     int
}

func (l *Leads) List(ctx context.Context, actor core.Actor, f ListFilter) ([]Lead, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := l.db.Reader().ListLeads(ctx, store.ListLeadsParams{
		WorkspaceID: actor.WorkspaceID, AccountID: optionalID(f.AccountID), Stage: nullString(f.Stage),
		StatusRaw: nullString(f.StatusRaw), AfterID: optionalID(f.AfterID), Max: int64(f.Limit),
	})
	if err != nil {
		return nil, err
	}
	mapping, err := l.currentFunnel(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Lead, 0, len(rows))
	for _, r := range rows {
		out = append(out, l.fromRow(r, mapping))
	}
	return out, nil
}

func (l *Leads) Coverage(ctx context.Context, actor core.Actor, accountID int64) (Coverage, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Coverage{}, core.ErrForbidden
	}
	q := l.db.Reader()
	row, err := q.LeadCoverage(ctx, store.LeadCoverageParams{WorkspaceID: actor.WorkspaceID, AccountID: optionalID(accountID)})
	if err != nil {
		return Coverage{}, err
	}
	shared, err := q.CountSharedPhones(ctx, actor.WorkspaceID)
	if err != nil {
		return Coverage{}, err
	}
	levels, err := q.CountAttributionLevels(ctx, actor.WorkspaceID)
	if err != nil {
		return Coverage{}, err
	}
	mapping, err := l.currentFunnel(ctx, actor.WorkspaceID)
	if err != nil {
		return Coverage{}, err
	}
	cov := Coverage{Total: row.Total, WithStatus: nullSum(row.WithStatus), WithStage: nullSum(row.WithStage), WithProject: nullSum(row.WithProject),
		WithPhone: nullSum(row.WithPhone), StatusMissing: nullSum(row.StatusMissing), SharedPhones: shared,
		MatchLevels: map[string]int64{}, FunnelConfigured: mapping != nil}
	for _, lv := range levels {
		cov.MatchLevels[lv.MatchLevel] = lv.N
	}
	return cov, nil
}

// ImportParams is a ghl.import run's params. Mode full reads every contact
// and, when it completes, marks contacts it didn't see as removed. Mode
// incremental reads contacts updated since Since and never removes anything.
type ImportParams struct {
	AccountID  int64      `json:"accountId"`
	LocationID string     `json:"locationId"`
	Mode       string     `json:"mode"`
	Since      *time.Time `json:"since"`
}

// ImportProgress is what the run records after each page, so a new attempt
// resumes from the cursor instead of the start.
type ImportProgress struct {
	Cursor   string `json:"cursor"`
	Pages    int    `json:"pages"`
	Contacts int    `json:"contacts"`
	Total    *int64 `json:"total"`
}

// RunKind is the run kind the importer handles.
const RunKind = "ghl.import"

// StartImport queues an import run for a connected location. The occurrence
// key is the location plus the mode plus the day, so one full import per day
// per location is one run however often it is asked for.
func (l *Leads) StartImport(ctx context.Context, actor core.Actor, r *runs.Runs, accountID int64, mode string, since *time.Time) (runs.Run, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return runs.Run{}, false, core.ErrForbidden
	}
	if mode != "full" && mode != "incremental" {
		return runs.Run{}, false, core.Invalid("import mode must be full or incremental")
	}
	acct, err := l.db.Reader().GetAccountByID(ctx, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return runs.Run{}, false, core.NotFound("account %d is not connected", accountID)
	}
	if err != nil {
		return runs.Run{}, false, err
	}
	if acct.Account.Type != "gohighlevel" {
		return runs.Run{}, false, core.Invalid("account %d is not a GoHighLevel location", accountID)
	}
	day := l.clock.Now().UTC().Format("2006-01-02")
	key := acct.Account.ExternalID + "/" + mode + "/" + day
	if mode == "incremental" && since != nil {
		key += "/" + since.UTC().Format(time.RFC3339)
	}
	return r.Start(ctx, actor, runs.StartInput{Kind: RunKind, OccurrenceKey: key, AccountID: accountID,
		Params: ImportParams{AccountID: accountID, LocationID: acct.Account.ExternalID, Mode: mode, Since: since}})
}

// Execute works one import run. It is the jobs.Executor for RunKind.
func (l *Leads) Execute(ctx context.Context, h *runs.Handle) (runs.Completion, error) {
	if l.crm == nil || l.tokens == nil {
		return runs.Completion{}, core.Conflict("GoHighLevel is not configured on this server")
	}
	var params ImportParams
	if err := json.Unmarshal(h.Run.Params, &params); err != nil {
		return runs.Completion{}, core.Invalid("import run has invalid parameters")
	}
	var progress ImportProgress
	if len(h.Run.Progress) > 0 {
		_ = json.Unmarshal(h.Run.Progress, &progress)
	}
	token, err := l.tokens.GHLToken(ctx, h.Actor, params.LocationID)
	if err != nil {
		return runs.Completion{}, err
	}
	mapping, err := l.currentFunnel(ctx, h.Actor.WorkspaceID)
	if err != nil {
		return runs.Completion{}, err
	}
	var since *time.Time
	if params.Mode == "incremental" {
		since = params.Since
	}
	for {
		page, err := l.crm.ListContacts(ctx, token, params.LocationID, progress.Cursor, since)
		if err != nil {
			// The pages already recorded stay. The run fails as partial
			// and a retry resumes from the cursor.
			return runs.Completion{}, err
		}
		for _, c := range page.Contacts {
			if _, err := l.ingest(ctx, h.Actor, h.Run.RowID(), params.AccountID, c, mapping); err != nil {
				return runs.Completion{}, err
			}
		}
		progress.Pages++
		progress.Contacts += len(page.Contacts)
		progress.Cursor = page.NextCursor
		if page.Total != nil {
			progress.Total = page.Total
		}
		now := l.clock.Now().UTC()
		if err := h.Progress(ctx, progress, &now); err != nil {
			return runs.Completion{}, err
		}
		if page.NextCursor == "" {
			break
		}
	}
	if params.Mode == "full" {
		// Only a scan that saw every page may say a contact is gone.
		now := l.clock.Now().UTC()
		if _, err := l.db.Writer().MarkLeadsUnseenRemoved(ctx, store.MarkLeadsUnseenRemovedParams{
			RemovedAt: sql.NullTime{Time: now, Valid: true}, AccountID: params.AccountID, RunID: sql.NullInt64{Int64: h.Run.RowID(), Valid: true},
		}); err != nil {
			return runs.Completion{}, err
		}
	}
	return runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}, nil
}

// Ingest applies one observed contact as actor (a job's system actor or an
// admin backfilling). Exported for tests and for the backfill CLI later.
func (l *Leads) Ingest(ctx context.Context, actor core.Actor, accountID int64, c Contact) (Lead, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Lead{}, core.ErrForbidden
	}
	mapping, err := l.currentFunnel(ctx, actor.WorkspaceID)
	if err != nil {
		return Lead{}, err
	}
	return l.ingest(ctx, actor, 0, accountID, c, mapping)
}

func (l *Leads) ingest(ctx context.Context, actor core.Actor, runID, accountID int64, c Contact, mapping *FunnelMapping) (Lead, error) {
	if c.ID == "" {
		return Lead{}, core.Invalid("contact has no id")
	}
	now := l.clock.Now().UTC()
	raw := c.Raw
	if raw == nil {
		raw, _ = json.Marshal(c)
	}
	ev, _, err := l.evidence.Record(ctx, actor, evidence.Observation{
		RunID: runID, AccountID: accountID, Source: "gohighlevel", Kind: "contact", SourceRef: c.ID,
		Scope: "account:" + strconv.FormatInt(accountID, 10), Class: evidence.Observed, EventAt: c.DateUpdated,
		Payload: json.RawMessage(raw), Completeness: evidence.Complete,
	})
	if err != nil {
		return Lead{}, err
	}

	var doc FunnelDocument
	var version sql.NullInt64
	if mapping != nil {
		doc = mapping.Document
		version = sql.NullInt64{Int64: mapping.Version, Valid: true}
	}
	previous, err := l.db.Reader().GetLeadByExternalID(ctx, store.GetLeadByExternalIDParams{AccountID: accountID, ExternalID: c.ID})
	known := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Lead{}, err
	}

	// Status: present in this observation, or missing. Missing keeps the
	// last known value and notes since when it has been missing.
	statusRaw, statusObserved, statusMissing := sql.NullString{}, sql.NullTime{}, sql.NullTime{}
	if value, present := doc.StatusField.read(c); present {
		statusRaw = nullString(strings.TrimSpace(value))
		statusObserved = sql.NullTime{Time: now, Valid: true}
	} else if known {
		statusRaw, statusObserved = previous.StatusRaw, previous.StatusObservedAt
		if previous.StatusMissingSince.Valid {
			statusMissing = previous.StatusMissingSince
		} else {
			statusMissing = sql.NullTime{Time: now, Valid: true}
		}
	} else if doc.StatusField.configured() {
		statusMissing = sql.NullTime{Time: now, Valid: true}
	}
	var stage *Stage
	if mapping != nil {
		var s *string
		if statusRaw.Valid {
			s = &statusRaw.String
		}
		stage = doc.stageFor(s)
	}

	projectRaw := sql.NullString{}
	projectID := sql.NullInt64{}
	if value, present := doc.ProjectField.read(c); present && strings.TrimSpace(value) != "" {
		projectRaw = nullString(strings.TrimSpace(value))
		if p, err := l.db.Reader().GetProjectByName(ctx, projectRaw.String); err == nil {
			projectID = sql.NullInt64{Int64: p.ID, Valid: true}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return Lead{}, err
		}
	}

	e164, dial := normalizePhone(c.Phone, c.Country)
	firstSeen := now
	if known {
		firstSeen = previous.FirstSeenAt
	}
	var row store.Lead
	err = l.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		row, err = q.UpsertLead(ctx, store.UpsertLeadParams{
			WorkspaceID: actor.WorkspaceID, AccountID: accountID, ExternalID: c.ID,
			FirstName: nullString(c.FirstName), LastName: nullString(c.LastName), Email: nullString(strings.ToLower(strings.TrimSpace(c.Email))),
			Phone: nullString(c.Phone), PhoneE164: nullString(e164), DialCode: nullString(dial),
			SourceRaw: nullString(c.Source), ProjectRaw: projectRaw, ProjectID: projectID,
			StatusRaw: statusRaw, StatusObservedAt: statusObserved, StatusMissingSince: statusMissing,
			Stage: nullStage(stage), MappingVersion: version,
			ProviderAddedAt: nullTimePtr(c.DateAdded), ProviderUpdatedAt: nullTimePtr(c.DateUpdated),
			FirstSeenAt: firstSeen, LastSeenAt: now, LastSeenRunID: optionalID(runID), EvidenceID: sql.NullInt64{Int64: ev.ID, Valid: true},
		})
		if err != nil {
			return err
		}
		for _, a := range c.Attributions {
			level, confidence, platform, ref := matchAttribution(a, doc)
			sum := sha256.Sum256([]byte(strings.Join([]string{a.UTMSource, a.UTMMedium, a.UTMCampaign, a.UTMContent, a.UTMTerm, a.CampaignID, a.AdID,
				a.GCLID, a.GBRAID, a.WBRAID, a.FBCLID, a.Referrer, a.LandingURL}, "\x00")))
			if err := q.InsertLeadAttribution(ctx, store.InsertLeadAttributionParams{
				LeadID: row.ID, ObservedAt: now, IsFirst: boolInt(a.IsFirst),
				UtmSource: nullString(a.UTMSource), UtmMedium: nullString(a.UTMMedium), UtmCampaign: nullString(a.UTMCampaign),
				UtmContent: nullString(a.UTMContent), UtmTerm: nullString(a.UTMTerm), Gclid: nullString(a.GCLID), Fbclid: nullString(a.FBCLID),
				Referrer: nullString(a.Referrer), LandingUrl: nullString(a.LandingURL),
				MatchLevel: level, MatchConfidence: confidence, MatchedPlatform: nullString(platform), MatchedCampaignRef: nullString(ref),
				PayloadHash: hex.EncodeToString(sum[:]),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Lead{}, err
	}
	return l.fromRow(row, mapping), nil
}

// matchAttribution grades how surely an attribution record ties the lead to
// a campaign (reference.md match levels). A click id is high confidence and
// names the platform. A UTM campaign is low: it is typed by hand and only
// campaign level. The campaign ref is left as text for B1.3 to resolve.
func matchAttribution(a Attribution, doc FunnelDocument) (level, confidence, platform, ref string) {
	switch {
	case a.GCLID != "":
		return "click_id", "high", "google", a.GCLID
	case a.GBRAID != "":
		return "click_id", "high", "google", a.GBRAID
	case a.WBRAID != "":
		return "click_id", "high", "google", a.WBRAID
	case a.FBCLID != "":
		return "click_id", "high", "meta", a.FBCLID
	case a.CampaignID != "":
		// The CRM recorded the platform's own campaign id: campaign level,
		// but not typed by hand.
		platform := doc.sourceFor(a.UTMSource)
		if platform == "" {
			platform = strings.ToLower(strings.TrimSpace(a.UTMSource))
		}
		return "utm", "medium", platform, a.CampaignID
	case a.UTMCampaign != "":
		platform = doc.sourceFor(a.UTMSource)
		if platform == "" {
			platform = strings.ToLower(strings.TrimSpace(a.UTMSource))
		}
		return "utm", "low", platform, a.UTMCampaign
	}
	return "none", "none", "", ""
}

// read pulls the configured value out of a contact. present is false when
// the field isn't configured or the contact doesn't carry it.
func (f FieldLocation) read(c Contact) (value string, present bool) {
	switch f.Kind {
	case "custom_field":
		v, ok := c.CustomFields[f.Ref]
		return v, ok
	case "contact_field":
		switch strings.ToLower(f.Ref) {
		case "source":
			return c.Source, true
		case "country":
			return c.Country, true
		}
		return "", false
	case "tag_prefix":
		for _, t := range c.Tags {
			if strings.HasPrefix(strings.ToLower(t), strings.ToLower(f.Ref)) {
				return strings.TrimSpace(t[len(f.Ref):]), true
			}
		}
		return "", false
	}
	return "", false
}

// dialCodes are the country calling codes that matter for Reportage's
// leads: the Gulf, India, Pakistan, Egypt, the UK and North America. Longer
// codes are tried first. The code says where someone lives, not their
// nationality (D-10).
var dialCodes = []string{"971", "966", "965", "974", "973", "968", "962", "961", "964", "963", "960", "880", "234", "254", "255", "27", "20", "91", "92", "93", "94", "98", "90", "44", "49", "33", "39", "34", "31", "32", "41", "43", "46", "47", "45", "48", "7", "81", "82", "86", "60", "61", "62", "63", "64", "65", "66", "84", "1"}

var nonDigits = regexp.MustCompile(`[^0-9]`)

// normalizePhone returns an E.164-style number and its dialing code, or
// empty strings when the input can't be read. A number without a leading +
// or 00 is assumed to be local to the contact's country when that is the
// UAE; otherwise it is kept as digits only with no code.
func normalizePhone(raw, country string) (e164, dial string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ""
	}
	international := strings.HasPrefix(s, "+") || strings.HasPrefix(s, "00")
	digits := nonDigits.ReplaceAllString(s, "")
	digits = strings.TrimPrefix(digits, "00")
	if len(digits) < 7 || len(digits) > 15 {
		return "", ""
	}
	if !international {
		if strings.HasPrefix(digits, "0") && (country == "" || strings.EqualFold(country, "AE")) {
			digits = "971" + digits[1:]
		} else if !strings.HasPrefix(digits, "971") && len(digits) == 9 && strings.EqualFold(country, "AE") {
			digits = "971" + digits
		}
	}
	for _, code := range dialCodes {
		if strings.HasPrefix(digits, code) && len(digits) > len(code)+4 {
			return "+" + digits, code
		}
	}
	return "+" + digits, ""
}

func (l *Leads) fromRow(r store.Lead, mapping *FunnelMapping) Lead {
	out := Lead{
		ID: r.ID, AccountID: r.AccountID, ExternalID: r.ExternalID,
		FirstName: nullStr(r.FirstName), LastName: nullStr(r.LastName), Email: nullStr(r.Email), Phone: nullStr(r.Phone),
		DialCode: nullStr(r.DialCode), SourceRaw: nullStr(r.SourceRaw), ProjectRaw: nullStr(r.ProjectRaw),
		StatusRaw: nullStr(r.StatusRaw), StatusObservedAt: timePtr(r.StatusObservedAt), StatusMissingSince: timePtr(r.StatusMissingSince),
		ProviderAddedAt: timePtr(r.ProviderAddedAt), ProviderUpdatedAt: timePtr(r.ProviderUpdatedAt),
		FirstSeenAt: r.FirstSeenAt.UTC(), LastSeenAt: r.LastSeenAt.UTC(),
	}
	if r.ProjectID.Valid {
		out.ProjectID = &r.ProjectID.Int64
	}
	if r.Stage.Valid {
		s := Stage(r.Stage.String)
		out.Stage = &s
	}
	if r.EvidenceID.Valid {
		out.EvidenceID = &r.EvidenceID.Int64
	}
	if mapping != nil && r.SourceRaw.Valid {
		if s := mapping.Document.sourceFor(r.SourceRaw.String); s != "" {
			out.Source = &s
		}
	}
	return out
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
func nullStr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}
func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
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
func nullSum(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case sql.NullInt64:
		return n.Int64
	case sql.NullFloat64:
		return int64(n.Float64)
	case float64:
		return int64(n)
	case nil:
		return 0
	}
	return 0
}
