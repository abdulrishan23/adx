// Package research gathers what a plan can be grounded on: pages read off
// the public web, competitor pages, Keyword Planner estimates and an audit
// of our own account history. Every retrieval is an item that either
// yields findings with their evidence or fails with a safe reason. Nothing
// is estimated by ADX: a source that didn't answer leaves a gap, a
// competitor named ambiguously stays unresolved, and figures from our own
// accounts are never mixed with what a competitor's website says.
package research

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/learning"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
)

// Provider contracts. Implementations live in server/providers.

type FetchKind string

const (
	// FetchRefused: ADX would not make the request (private network, bad
	// scheme). Not retried.
	FetchRefused FetchKind = "refused"
	// FetchUnavailable: the source didn't answer usefully. Retried later.
	FetchUnavailable FetchKind = "unavailable"
	// FetchUnsupported: not a web page.
	FetchUnsupported FetchKind = "unsupported"
)

type FetchError struct {
	Kind    FetchKind
	Message string
}

func (e FetchError) Error() string { return string(e.Kind) + ": " + e.Message }

// Page is a fetched web page reduced to text.
type Page struct {
	URL         string
	FinalURL    string
	Status      int
	ContentType string
	Title       string
	Text        string
	Truncated   bool
	FetchedAt   time.Time
}

type Fetcher interface {
	Fetch(ctx context.Context, url string) (Page, error)
}

// KeywordIdea is one Keyword Planner estimate. Nil means Google gave no
// figure.
type KeywordIdea struct {
	Text                  string `json:"text"`
	AvgMonthlySearches    *int64 `json:"avgMonthlySearches"`
	Competition           string `json:"competition"`
	CompetitionIndex      *int64 `json:"competitionIndex"`
	LowTopOfPageBidMinor  *int64 `json:"lowTopOfPageBidMinor"`
	HighTopOfPageBidMinor *int64 `json:"highTopOfPageBidMinor"`
}

type KeywordProvider interface {
	KeywordIdeas(ctx context.Context, token, customerID string, seeds []string, country, language string) ([]KeywordIdea, error)
}

// Analytics is the deterministic reporting engine, for the account audit.
type Analytics interface {
	Analytics(ctx context.Context, actor core.Actor, q reporting.AnalyticsQuery) (reporting.ReportingResult, error)
}

// Domain types.

type Brief struct {
	ID                 int64     `json:"id"`
	BrandID            int64     `json:"brandId"`
	ProjectID          *int64    `json:"projectId"`
	Title              string    `json:"title"`
	Question           string    `json:"question"`
	Geography          string    `json:"geography"`
	Language           string    `json:"language"`
	GoogleAdsAccountID *int64    `json:"googleAdsAccountId"`
	CreatedBy          *int64    `json:"createdBy"`
	CreatedAt          time.Time `json:"createdAt"`
}

type CompetitorInput struct {
	Name string   `json:"name"`
	URLs []string `json:"urls"`
}

type BriefInput struct {
	BrandID   int64  `json:"brandId"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	Question  string `json:"question"`
	// Geography is an ISO country code such as AE.
	Geography string `json:"geography"`
	Language  string `json:"language"`
	// URLs are our own or market pages to read.
	URLs        []string          `json:"urls"`
	Competitors []CompetitorInput `json:"competitors"`
	// KeywordSeeds are sized with Keyword Planner through the Google Ads
	// account named here. Without an account the item fails as unconfigured.
	KeywordSeeds       []string `json:"keywordSeeds"`
	GoogleAdsAccountID int64    `json:"googleAdsAccountId"`
	// AuditAccount adds an item that reads our own last 90 days.
	AuditAccount bool `json:"auditAccount"`
}

type ItemKind string

const (
	ItemWebPage        ItemKind = "web_page"
	ItemCompetitorPage ItemKind = "competitor_page"
	ItemKeywords       ItemKind = "keywords"
	ItemAccountAudit   ItemKind = "account_audit"
)

type ItemState string

const (
	ItemPending       ItemState = "pending"
	ItemNeedsIdentity ItemState = "needs_identity"
	ItemFetched       ItemState = "fetched"
	ItemFailed        ItemState = "failed"
	ItemRefused       ItemState = "refused"
)

type IdentityState string

const (
	IdentityNA        IdentityState = "n/a"
	IdentityResolved  IdentityState = "resolved"
	IdentityAmbiguous IdentityState = "ambiguous"
	IdentityUnknown   IdentityState = "unknown"
)

type BrandRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Item struct {
	ID                int64         `json:"id"`
	BriefID           int64         `json:"briefId"`
	Kind              ItemKind      `json:"kind"`
	Target            string        `json:"target"`
	CompetitorName    string        `json:"competitorName"`
	CompetitorBrandID *int64        `json:"competitorBrandId"`
	IdentityState     IdentityState `json:"identityState"`
	Candidates        []BrandRef    `json:"candidates"`
	State             ItemState     `json:"state"`
	Error             string        `json:"error"`
	Attempts          int64         `json:"attempts"`
	FetchedAt         *time.Time    `json:"fetchedAt"`
	RunID             *int64        `json:"runId"`
	EvidenceID        *int64        `json:"evidenceId"`
	CreatedAt         time.Time     `json:"createdAt"`
}

type Scope string

const (
	// ScopeAccount is our own performance data. Private.
	ScopeAccount Scope = "account"
	// ScopePublic is anything read off the web, ours or a competitor's.
	ScopePublic Scope = "public"
)

type FindingKind string

const (
	FindingPageText    FindingKind = "page_text"
	FindingKeyword     FindingKind = "keyword"
	FindingAuditMetric FindingKind = "audit_metric"
	FindingAuditNote   FindingKind = "audit_note"
)

type Finding struct {
	ID               int64           `json:"id"`
	BriefID          int64           `json:"briefId"`
	ItemID           int64           `json:"itemId"`
	Scope            Scope           `json:"scope"`
	SubjectBrandID   *int64          `json:"subjectBrandId"`
	Kind             FindingKind     `json:"kind"`
	Class            evidence.Class  `json:"class"`
	Label            string          `json:"label"`
	Payload          json.RawMessage `json:"payload"`
	SourceURL        string          `json:"sourceUrl"`
	SourceObservedAt time.Time       `json:"sourceObservedAt"`
	Geography        string          `json:"geography"`
	EvidenceID       *int64          `json:"evidenceId"`
	CreatedAt        time.Time       `json:"createdAt"`
}

// CompetitorStatus is one named competitor and whether ADX knows who it is.
type CompetitorStatus struct {
	Name       string        `json:"name"`
	State      IdentityState `json:"state"`
	BrandID    *int64        `json:"brandId"`
	Candidates []BrandRef    `json:"candidates"`
}

type BriefDetail struct {
	Brief       Brief              `json:"brief"`
	Items       []Item             `json:"items"`
	Findings    []Finding          `json:"findings"`
	Competitors []CompetitorStatus `json:"competitors"`
	Runs        int64              `json:"runs"`
}

// Memory is what research asks of organizational memory: the validated and
// supported findings to carry into a report.
type Memory interface {
	Findings(ctx context.Context, actor core.Actor) ([]learning.MemoryFinding, error)
}

type Config struct {
	Clock     core.Clock
	Fetcher   Fetcher
	Keywords  KeywordProvider
	Tokens    reporting.Tokens
	Analytics Analytics
	Memory    Memory
}

type Research struct {
	db        *store.DB
	clock     core.Clock
	memory    Memory
	fetcher   Fetcher
	keywords  KeywordProvider
	tokens    reporting.Tokens
	analytics Analytics
	evidence  *evidence.Store
	brands    *brands.Brands
}

func New(db *store.DB, ev *evidence.Store, br *brands.Brands, cfg Config) *Research {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Research{db: db, clock: cfg.Clock, memory: cfg.Memory, fetcher: cfg.Fetcher, keywords: cfg.Keywords, tokens: cfg.Tokens, analytics: cfg.Analytics, evidence: ev, brands: br}
}

// ResearchRunKind is the run kind the research worker handles.
const ResearchRunKind = "research.run"

type RunParams struct {
	BriefID int64 `json:"briefId"`
	// Mode is "" (pending and failed items only) or "refresh" (re-read every
	// item that can be read, so a scheduled comparison sees today's pages).
	Mode string `json:"mode"`
}

type RunProgress struct {
	Total   int `json:"total"`
	Done    int `json:"done"`
	Failed  int `json:"failed"`
	Refused int `json:"refused"`
	Waiting int `json:"waiting"`
}

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

// CreateBrief records the question and its items. Marketers may. Competitor
// names are matched against competitor brands: an exact name resolves, a
// partial match or none leaves the item waiting for a person. URLs that
// ADX would refuse to fetch are refused here too, so the person sees it now.
func (r *Research) CreateBrief(ctx context.Context, actor core.Actor, in BriefInput) (BriefDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return BriefDetail{}, core.ErrForbidden
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 160 {
		return BriefDetail{}, core.Invalid("a title of up to 160 characters is required")
	}
	in.Geography = strings.ToUpper(strings.TrimSpace(in.Geography))
	if in.Geography == "" {
		in.Geography = "AE"
	}
	if !countryPattern.MatchString(in.Geography) {
		return BriefDetail{}, core.Invalid("geography must be a two-letter country code such as AE")
	}
	in.Language = strings.ToLower(strings.TrimSpace(in.Language))
	if in.Language == "" {
		in.Language = "en"
	}
	if in.Language != "en" && in.Language != "ar" {
		return BriefDetail{}, core.Invalid("language must be en or ar")
	}
	if in.BrandID == 0 {
		return BriefDetail{}, core.Invalid("a brand is required")
	}
	if _, err := r.brands.Get(ctx, actor, in.BrandID); err != nil {
		return BriefDetail{}, err
	}
	if in.ProjectID != 0 {
		owner, err := r.brands.BrandOfProject(ctx, actor, in.ProjectID)
		if err != nil {
			return BriefDetail{}, err
		}
		if owner != in.BrandID {
			return BriefDetail{}, core.Invalid("project %d does not belong to brand %d", in.ProjectID, in.BrandID)
		}
	}
	if in.GoogleAdsAccountID != 0 {
		acct, err := r.db.Reader().GetAccountByID(ctx, in.GoogleAdsAccountID)
		if errors.Is(err, sql.ErrNoRows) {
			return BriefDetail{}, core.NotFound("account %d is not connected", in.GoogleAdsAccountID)
		}
		if err != nil {
			return BriefDetail{}, err
		}
		if acct.Account.Type != "google_ads" {
			return BriefDetail{}, core.Invalid("keyword ideas need a Google Ads account")
		}
	}
	urls, err := r.cleanURLs(ctx, in.URLs)
	if err != nil {
		return BriefDetail{}, err
	}
	type compPlan struct {
		CompetitorInput
		state      IdentityState
		brandID    int64
		candidates []BrandRef
	}
	var comps []compPlan
	competitorBrands, err := r.db.Reader().ListCompetitorBrands(ctx, actor.WorkspaceID)
	if err != nil {
		return BriefDetail{}, err
	}
	for _, c := range in.Competitors {
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" {
			continue
		}
		cu, err := r.cleanURLs(ctx, c.URLs)
		if err != nil {
			return BriefDetail{}, err
		}
		c.URLs = cu
		state, brandID, cands := resolveCompetitor(c.Name, competitorBrands)
		comps = append(comps, compPlan{CompetitorInput: c, state: state, brandID: brandID, candidates: cands})
	}
	seeds := make([]string, 0, len(in.KeywordSeeds))
	for _, s := range in.KeywordSeeds {
		if s = strings.TrimSpace(s); s != "" {
			seeds = append(seeds, s)
		}
	}
	if len(seeds) > 20 {
		return BriefDetail{}, core.Invalid("at most 20 keyword seeds")
	}
	if len(urls)+len(comps)+len(seeds) == 0 && !in.AuditAccount {
		return BriefDetail{}, core.Invalid("a brief needs at least one page, competitor, keyword seed or the account audit")
	}
	now := r.clock.Now().UTC()
	var brow store.ResearchBrief
	err = r.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		brow, err = q.CreateResearchBrief(ctx, store.CreateResearchBriefParams{
			WorkspaceID: actor.WorkspaceID, BrandID: in.BrandID, ProjectID: optionalID(in.ProjectID), Title: in.Title, Question: strings.TrimSpace(in.Question),
			Geography: in.Geography, Language: in.Language, GoogleAdsAccountID: optionalID(in.GoogleAdsAccountID), CreatedBy: optionalID(actor.UserID), CreatedAt: now,
		})
		if err != nil {
			return err
		}
		add := func(p store.CreateResearchItemParams) error {
			p.WorkspaceID, p.BriefID, p.CreatedAt = actor.WorkspaceID, brow.ID, now
			if p.Candidates == "" {
				p.Candidates = "[]"
			}
			_, err := q.CreateResearchItem(ctx, p)
			return err
		}
		for _, u := range urls {
			if err := add(store.CreateResearchItemParams{Kind: string(ItemWebPage), Target: u, IdentityState: string(IdentityNA), State: string(ItemPending)}); err != nil {
				return err
			}
		}
		for _, c := range comps {
			cands, _ := json.Marshal(c.candidates)
			state := ItemPending
			if c.state != IdentityResolved {
				state = ItemNeedsIdentity
			}
			targets := c.URLs
			if len(targets) == 0 {
				// A competitor with no page to read is still recorded, so the
				// identity question is visible and later tasks can add pages.
				targets = []string{""}
			}
			for _, u := range targets {
				if err := add(store.CreateResearchItemParams{Kind: string(ItemCompetitorPage), Target: u, CompetitorName: c.Name, CompetitorBrandID: optionalID(c.brandID),
					IdentityState: string(c.state), Candidates: string(cands), State: string(state)}); err != nil {
					return err
				}
			}
		}
		if len(seeds) > 0 {
			if err := add(store.CreateResearchItemParams{Kind: string(ItemKeywords), Target: strings.Join(seeds, "\n"), IdentityState: string(IdentityNA), State: string(ItemPending)}); err != nil {
				return err
			}
		}
		if in.AuditAccount {
			if err := add(store.CreateResearchItemParams{Kind: string(ItemAccountAudit), Target: "last 90 days", IdentityState: string(IdentityNA), State: string(ItemPending)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return BriefDetail{}, err
	}
	return r.GetBrief(ctx, actor, brow.ID)
}

// cleanURLs trims, dedupes and refuses what the fetcher would refuse.
func (r *Research) cleanURLs(ctx context.Context, in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, core.Invalid("%q is not a web address", raw)
		}
		if v, ok := r.fetcher.(interface {
			Validate(context.Context, string) error
		}); ok {
			var fe FetchError
			if err := v.Validate(ctx, raw); errors.As(err, &fe) && fe.Kind == FetchRefused {
				return nil, core.Invalid("%s: %s", raw, fe.Message)
			}
		}
		seen[raw] = true
		out = append(out, raw)
	}
	if len(out) > 50 {
		return nil, core.Invalid("at most 50 pages per brief")
	}
	return out, nil
}

// resolveCompetitor matches a typed name to competitor brands. Only an exact
// name (ignoring case and spacing) resolves; anything looser is a guess and
// is left for a person, with the near matches listed.
func resolveCompetitor(name string, brandRows []store.Brand) (IdentityState, int64, []BrandRef) {
	norm := func(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
	n := norm(name)
	var cands []BrandRef
	for _, b := range brandRows {
		bn := norm(b.Name)
		if bn == n {
			return IdentityResolved, b.ID, nil
		}
		if strings.Contains(bn, n) || strings.Contains(n, bn) {
			cands = append(cands, BrandRef{ID: b.ID, Name: b.Name})
		}
	}
	if len(cands) > 0 {
		return IdentityAmbiguous, 0, cands
	}
	return IdentityUnknown, 0, []BrandRef{}
}

// ResolveCompetitor settles who a named competitor is: a competitor brand
// in this workspace. Marketers may. Items waiting on the name become
// pending.
func (r *Research) ResolveCompetitor(ctx context.Context, actor core.Actor, briefID int64, name string, brandID int64) (BriefDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return BriefDetail{}, core.ErrForbidden
	}
	if _, err := r.brief(ctx, actor.WorkspaceID, briefID); err != nil {
		return BriefDetail{}, err
	}
	b, err := r.brands.Get(ctx, actor, brandID)
	if err != nil {
		return BriefDetail{}, err
	}
	if b.Brand.Kind != brands.Competitor {
		return BriefDetail{}, core.Invalid("%q is not a competitor brand", b.Brand.Name)
	}
	n, err := r.db.Writer().ResolveResearchItemsCompetitor(ctx, store.ResolveResearchItemsCompetitorParams{CompetitorBrandID: optionalID(brandID), BriefID: briefID, CompetitorName: strings.TrimSpace(name)})
	if err != nil {
		return BriefDetail{}, err
	}
	if n == 0 {
		return BriefDetail{}, core.NotFound("no item in this brief is waiting on competitor %q", name)
	}
	return r.GetBrief(ctx, actor, briefID)
}

func (r *Research) ListBriefs(ctx context.Context, actor core.Actor) ([]Brief, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := r.db.Reader().ListResearchBriefs(ctx, store.ListResearchBriefsParams{WorkspaceID: actor.WorkspaceID, Limit: 200})
	if err != nil {
		return nil, err
	}
	out := make([]Brief, 0, len(rows))
	for _, b := range rows {
		out = append(out, briefFromRow(b))
	}
	return out, nil
}

func (r *Research) GetBrief(ctx context.Context, actor core.Actor, id int64) (BriefDetail, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return BriefDetail{}, core.ErrForbidden
	}
	brow, err := r.brief(ctx, actor.WorkspaceID, id)
	if err != nil {
		return BriefDetail{}, err
	}
	d := BriefDetail{Brief: briefFromRow(brow), Items: []Item{}, Findings: []Finding{}, Competitors: []CompetitorStatus{}}
	items, err := r.db.Reader().ListResearchItems(ctx, id)
	if err != nil {
		return BriefDetail{}, err
	}
	seen := map[string]bool{}
	for _, it := range items {
		item := itemFromRow(it)
		d.Items = append(d.Items, item)
		if item.Kind == ItemCompetitorPage && !seen[strings.ToLower(item.CompetitorName)] {
			seen[strings.ToLower(item.CompetitorName)] = true
			d.Competitors = append(d.Competitors, CompetitorStatus{Name: item.CompetitorName, State: item.IdentityState, BrandID: item.CompetitorBrandID, Candidates: item.Candidates})
		}
	}
	d.Findings, err = r.ListFindings(ctx, actor, id, "")
	if err != nil {
		return BriefDetail{}, err
	}
	d.Runs, err = r.db.Reader().CountResearchRunsForBrief(ctx, id)
	if err != nil {
		return BriefDetail{}, err
	}
	return d, nil
}

// ListFindings is a brief's findings, optionally one scope. Account figures
// and public observations are never summed together; the caller gets them
// apart and shows them apart.
func (r *Research) ListFindings(ctx context.Context, actor core.Actor, briefID int64, scope Scope) ([]Finding, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if _, err := r.brief(ctx, actor.WorkspaceID, briefID); err != nil {
		return nil, err
	}
	rows, err := r.db.Reader().ListResearchFindings(ctx, store.ListResearchFindingsParams{BriefID: briefID, Scope: sql.NullString{String: string(scope), Valid: scope != ""}})
	if err != nil {
		return nil, err
	}
	out := make([]Finding, 0, len(rows))
	for _, f := range rows {
		out = append(out, findingFromRow(f))
	}
	return out, nil
}

// Evidence is the stored observation behind a finding, for drill-down.
func (r *Research) Evidence(ctx context.Context, actor core.Actor, id int64) (evidence.Evidence, error) {
	return r.evidence.Get(ctx, actor, id)
}

// StartRun queues the brief's pending items. One run per brief per day;
// starting again the same day returns that run.
func (r *Research) StartRun(ctx context.Context, actor core.Actor, rs *runs.Runs, briefID int64) (runs.Run, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return runs.Run{}, false, core.ErrForbidden
	}
	if _, err := r.brief(ctx, actor.WorkspaceID, briefID); err != nil {
		return runs.Run{}, false, err
	}
	key := fmt.Sprintf("brief:%d/%s", briefID, r.clock.Now().UTC().Format("2006-01-02"))
	return rs.Start(ctx, actor, runs.StartInput{Kind: ResearchRunKind, OccurrenceKey: key, Params: RunParams{BriefID: briefID}})
}

// Execute works one research run: every pending or failed item once, or
// every readable item in refresh mode. It is the jobs.Executor for
// ResearchRunKind. Items waiting on a competitor's identity are skipped.
// The run is partial when anything was skipped or failed.
func (r *Research) Execute(ctx context.Context, h *runs.Handle) (runs.Completion, error) {
	var params RunParams
	if err := json.Unmarshal(h.Run.Params, &params); err != nil || params.BriefID == 0 {
		return runs.Completion{}, core.Invalid("research run has invalid parameters")
	}
	brow, err := r.brief(ctx, h.Actor.WorkspaceID, params.BriefID)
	if err != nil {
		return runs.Completion{}, err
	}
	items, err := r.db.Reader().ListResearchItems(ctx, params.BriefID)
	if err != nil {
		return runs.Completion{}, err
	}
	progress := RunProgress{Total: len(items)}
	for _, it := range items {
		switch ItemState(it.State) {
		case ItemNeedsIdentity:
			progress.Waiting++
			continue
		case ItemRefused:
			// A refusal is deterministic; running again changes nothing.
			progress.Refused++
			continue
		case ItemFetched:
			if params.Mode != "refresh" && it.RunID.Valid && it.RunID.Int64 != h.Run.RowID() {
				// Fetched by an earlier run; leave it unless this is a refresh.
				progress.Done++
				continue
			}
		}
		ferr := r.workItem(ctx, h, brow, it)
		now := r.clock.Now().UTC()
		var fe FetchError
		switch {
		case ferr == nil:
			progress.Done++
		case errors.As(ferr, &fe) && fe.Kind == FetchRefused:
			progress.Refused++
			if err := r.db.Writer().MarkResearchItemFailed(ctx, store.MarkResearchItemFailedParams{State: string(ItemRefused), ErrorSafe: fe.Message, RunID: sql.NullInt64{Int64: h.Run.RowID(), Valid: true}, ID: it.ID}); err != nil {
				return runs.Completion{}, err
			}
		default:
			progress.Failed++
			if err := r.db.Writer().MarkResearchItemFailed(ctx, store.MarkResearchItemFailedParams{State: string(ItemFailed), ErrorSafe: safeMessage(ferr), RunID: sql.NullInt64{Int64: h.Run.RowID(), Valid: true}, ID: it.ID}); err != nil {
				return runs.Completion{}, err
			}
		}
		if err := h.Progress(ctx, progress, &now); err != nil {
			return runs.Completion{}, err
		}
	}
	if progress.Failed+progress.Refused+progress.Waiting == 0 {
		return runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}, nil
	}
	var parts []string
	if progress.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d source(s) unavailable", progress.Failed))
	}
	if progress.Refused > 0 {
		parts = append(parts, fmt.Sprintf("%d address(es) refused", progress.Refused))
	}
	if progress.Waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d item(s) waiting for a competitor's identity", progress.Waiting))
	}
	return runs.Completion{State: runs.StatePartial, Completeness: runs.CompletenessPartial, Error: strings.Join(parts, "; ")}, nil
}

// safeMessage is what a failure shows. Provider errors keep their own
// wording only when they are ours (core.Error or FetchError).
func safeMessage(err error) string {
	var fe FetchError
	if errors.As(err, &fe) {
		return fe.Message
	}
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Message
	}
	return "the source could not be read"
}

func (r *Research) workItem(ctx context.Context, h *runs.Handle, brief store.ResearchBrief, it store.ResearchItem) error {
	switch ItemKind(it.Kind) {
	case ItemWebPage, ItemCompetitorPage:
		return r.workPage(ctx, h, brief, it)
	case ItemKeywords:
		return r.workKeywords(ctx, h, brief, it)
	case ItemAccountAudit:
		return r.workAudit(ctx, h, brief, it)
	}
	return core.Invalid("unknown research item kind %q", it.Kind)
}

// workPage reads one page and records it. The page's text is the finding;
// nothing in it is interpreted, followed or executed.
func (r *Research) workPage(ctx context.Context, h *runs.Handle, brief store.ResearchBrief, it store.ResearchItem) error {
	if r.fetcher == nil {
		return core.Conflict("web fetching is not configured on this server")
	}
	if strings.TrimSpace(it.Target) == "" {
		return core.Invalid("no page to read for this competitor yet")
	}
	page, err := r.fetcher.Fetch(ctx, it.Target)
	if err != nil {
		return err
	}
	subject := optionalID(brief.BrandID)
	if ItemKind(it.Kind) == ItemCompetitorPage {
		subject = it.CompetitorBrandID
	}
	const maxText = 60_000
	text := page.Text
	if len(text) > maxText {
		text = text[:maxText]
		page.Truncated = true
	}
	payload := map[string]any{"url": page.URL, "finalUrl": page.FinalURL, "status": page.Status, "title": page.Title, "text": text, "truncated": page.Truncated, "chars": len(text)}
	completeness := evidence.Complete
	if page.Truncated {
		completeness = evidence.Partial
	}
	ev, _, err := r.evidence.Record(ctx, h.Actor, evidence.Observation{
		RunID: h.Run.RowID(), Source: "web", Kind: "page", SourceRef: page.URL, Scope: fmt.Sprintf("brief:%d", brief.ID), Class: evidence.Observed, Payload: payload, Completeness: completeness,
	})
	if err != nil {
		return err
	}
	now := r.clock.Now().UTC()
	summary := map[string]any{"title": page.Title, "excerpt": excerpt(text, 600), "chars": len(text), "truncated": page.Truncated, "finalUrl": page.FinalURL}
	raw, _ := json.Marshal(summary)
	label := page.Title
	if label == "" {
		label = page.URL
	}
	return r.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if err := q.DeleteResearchFindingsForItem(ctx, it.ID); err != nil {
			return err
		}
		if _, err := q.CreateResearchFinding(ctx, store.CreateResearchFindingParams{
			WorkspaceID: h.Actor.WorkspaceID, BriefID: brief.ID, ItemID: it.ID, Scope: string(ScopePublic), SubjectBrandID: subject, Kind: string(FindingPageText), Class: string(evidence.Observed),
			Label: label, Payload: string(raw), SourceUrl: page.URL, SourceObservedAt: now, Geography: brief.Geography, EvidenceID: sql.NullInt64{Int64: ev.ID, Valid: true}, CreatedAt: now,
		}); err != nil {
			return err
		}
		return q.MarkResearchItemFetched(ctx, store.MarkResearchItemFetchedParams{FetchedAt: sql.NullTime{Time: now, Valid: true}, RunID: sql.NullInt64{Int64: h.Run.RowID(), Valid: true}, EvidenceID: sql.NullInt64{Int64: ev.ID, Valid: true}, ID: it.ID})
	})
}

// workKeywords sizes the seeds with Keyword Planner. Every figure is
// Google's estimate and is stored as such.
func (r *Research) workKeywords(ctx context.Context, h *runs.Handle, brief store.ResearchBrief, it store.ResearchItem) error {
	if r.keywords == nil || r.tokens == nil {
		return core.Conflict("keyword ideas are not configured on this server")
	}
	if !brief.GoogleAdsAccountID.Valid {
		return core.Conflict("no Google Ads account is chosen for keyword ideas on this brief")
	}
	acct, err := r.db.Reader().GetAccountByID(ctx, brief.GoogleAdsAccountID.Int64)
	if err != nil {
		return core.Conflict("the Google Ads account for keyword ideas is not connected")
	}
	token, err := r.tokens.ProviderToken(ctx, h.Actor, brief.GoogleAdsAccountID.Int64)
	if err != nil {
		return err
	}
	seeds := strings.Split(it.Target, "\n")
	ideas, err := r.keywords.KeywordIdeas(ctx, token, acct.Account.ExternalID, seeds, brief.Geography, brief.Language)
	if err != nil {
		return err
	}
	currency := ""
	if acct.Account.Currency.Valid {
		currency = acct.Account.Currency.String
	}
	ev, _, err := r.evidence.Record(ctx, h.Actor, evidence.Observation{
		RunID: h.Run.RowID(), AccountID: brief.GoogleAdsAccountID.Int64, Source: "google_ads", Kind: "keyword_ideas",
		SourceRef: fmt.Sprintf("%s/%s/%s/%s", acct.Account.ExternalID, brief.Geography, brief.Language, strings.Join(seeds, "|")), Scope: fmt.Sprintf("brief:%d", brief.ID),
		Class: evidence.Estimated, Payload: map[string]any{"seeds": seeds, "country": brief.Geography, "language": brief.Language, "currency": currency, "ideas": ideas}, Completeness: evidence.Complete,
	})
	if err != nil {
		return err
	}
	now := r.clock.Now().UTC()
	return r.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if err := q.DeleteResearchFindingsForItem(ctx, it.ID); err != nil {
			return err
		}
		for _, k := range ideas {
			raw, _ := json.Marshal(map[string]any{"idea": k, "currency": currency, "source": "Google Keyword Planner", "note": "Google's estimate, not an observation"})
			if _, err := q.CreateResearchFinding(ctx, store.CreateResearchFindingParams{
				WorkspaceID: h.Actor.WorkspaceID, BriefID: brief.ID, ItemID: it.ID, Scope: string(ScopePublic), SubjectBrandID: sql.NullInt64{}, Kind: string(FindingKeyword), Class: string(evidence.Estimated),
				Label: k.Text, Payload: string(raw), SourceUrl: "", SourceObservedAt: now, Geography: brief.Geography, EvidenceID: sql.NullInt64{Int64: ev.ID, Valid: true}, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		return q.MarkResearchItemFetched(ctx, store.MarkResearchItemFetchedParams{FetchedAt: sql.NullTime{Time: now, Valid: true}, RunID: sql.NullInt64{Int64: h.Run.RowID(), Valid: true}, EvidenceID: sql.NullInt64{Int64: ev.ID, Valid: true}, ID: it.ID})
	})
}

// workAudit reads our own last 90 days through the reporting engine, per
// platform. With no history the finding says so; no figure is invented.
func (r *Research) workAudit(ctx context.Context, h *runs.Handle, brief store.ResearchBrief, it store.ResearchItem) error {
	if r.analytics == nil {
		return core.Conflict("analytics are not configured on this server")
	}
	now := r.clock.Now().In(reporting.ReportingZone)
	to := now.AddDate(0, 0, -1)
	from := to.AddDate(0, 0, -89)
	q := reporting.AnalyticsQuery{From: reporting.DayOf(from), To: reporting.DayOf(to), ProjectID: brief.ProjectID.Int64, GroupBy: "platform"}
	res, err := r.analytics.Analytics(ctx, h.Actor, q)
	if err != nil {
		return err
	}
	completeness := evidence.Complete
	if res.Freshness.Partial {
		completeness = evidence.Partial
	}
	ev, _, err := r.evidence.Record(ctx, h.Actor, evidence.Observation{
		RunID: h.Run.RowID(), Source: "adx", Kind: "account_audit", SourceRef: fmt.Sprintf("brief:%d/%s..%s", brief.ID, q.From, q.To), Scope: fmt.Sprintf("brief:%d", brief.ID),
		Class: evidence.Observed, Payload: res, Completeness: completeness,
	})
	if err != nil {
		return err
	}
	at := r.clock.Now().UTC()
	return r.db.Tx(ctx, func(_ *sql.Tx, qq *store.Queries) error {
		if err := qq.DeleteResearchFindingsForItem(ctx, it.ID); err != nil {
			return err
		}
		add := func(kind FindingKind, label string, payload any) error {
			raw, _ := json.Marshal(payload)
			_, err := qq.CreateResearchFinding(ctx, store.CreateResearchFindingParams{
				WorkspaceID: h.Actor.WorkspaceID, BriefID: brief.ID, ItemID: it.ID, Scope: string(ScopeAccount), SubjectBrandID: optionalID(brief.BrandID), Kind: string(kind), Class: string(evidence.Observed),
				Label: label, Payload: string(raw), SourceObservedAt: at, Geography: brief.Geography, EvidenceID: sql.NullInt64{Int64: ev.ID, Valid: true}, CreatedAt: at,
			})
			return err
		}
		hasData := res.Summary.Metrics.Spend.Minor != nil || res.Summary.Metrics.Leads > 0 || len(res.Rows) > 0
		if !hasData {
			if err := add(FindingAuditNote, "limited audit: no account history for this scope", map[string]any{"period": res.Period, "coverage": res.Coverage, "warnings": res.Warnings, "note": "No ad spend or leads were recorded for this scope in the period, so no performance baseline exists. Nothing was estimated."}); err != nil {
				return err
			}
		} else {
			if err := add(FindingAuditMetric, "all platforms", map[string]any{"period": res.Period, "metrics": res.Summary.Metrics, "coverage": res.Coverage, "freshness": res.Freshness, "formulaVersion": res.FormulaVersion}); err != nil {
				return err
			}
			for _, row := range res.Rows {
				if err := add(FindingAuditMetric, row.Label, map[string]any{"period": res.Period, "platform": row.Platform, "metrics": row.Metrics, "formulaVersion": res.FormulaVersion}); err != nil {
					return err
				}
			}
		}
		for _, w := range res.Warnings {
			if err := add(FindingAuditNote, w.Code, map[string]any{"message": w.Message}); err != nil {
				return err
			}
		}
		return qq.MarkResearchItemFetched(ctx, store.MarkResearchItemFetchedParams{FetchedAt: sql.NullTime{Time: at, Valid: true}, RunID: sql.NullInt64{Int64: h.Run.RowID(), Valid: true}, EvidenceID: sql.NullInt64{Int64: ev.ID, Valid: true}, ID: it.ID})
	})
}

func (r *Research) brief(ctx context.Context, workspaceID, id int64) (store.ResearchBrief, error) {
	row, err := r.db.Reader().GetResearchBrief(ctx, store.GetResearchBriefParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.ResearchBrief{}, core.NotFound("research brief %d not found", id)
	}
	return row, err
}

func excerpt(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

func briefFromRow(b store.ResearchBrief) Brief {
	return Brief{ID: b.ID, BrandID: b.BrandID, ProjectID: idPtr(b.ProjectID), Title: b.Title, Question: b.Question, Geography: b.Geography, Language: b.Language,
		GoogleAdsAccountID: idPtr(b.GoogleAdsAccountID), CreatedBy: idPtr(b.CreatedBy), CreatedAt: b.CreatedAt.UTC()}
}

func itemFromRow(it store.ResearchItem) Item {
	item := Item{ID: it.ID, BriefID: it.BriefID, Kind: ItemKind(it.Kind), Target: it.Target, CompetitorName: it.CompetitorName, CompetitorBrandID: idPtr(it.CompetitorBrandID),
		IdentityState: IdentityState(it.IdentityState), Candidates: []BrandRef{}, State: ItemState(it.State), Error: it.ErrorSafe, Attempts: it.Attempts, RunID: idPtr(it.RunID), EvidenceID: idPtr(it.EvidenceID), CreatedAt: it.CreatedAt.UTC()}
	_ = json.Unmarshal([]byte(it.Candidates), &item.Candidates)
	sort.Slice(item.Candidates, func(i, j int) bool { return item.Candidates[i].Name < item.Candidates[j].Name })
	if it.FetchedAt.Valid {
		t := it.FetchedAt.Time.UTC()
		item.FetchedAt = &t
	}
	return item
}

func findingFromRow(f store.ResearchFinding) Finding {
	return Finding{ID: f.ID, BriefID: f.BriefID, ItemID: f.ItemID, Scope: Scope(f.Scope), SubjectBrandID: idPtr(f.SubjectBrandID), Kind: FindingKind(f.Kind), Class: evidence.Class(f.Class), Label: f.Label,
		Payload: json.RawMessage(f.Payload), SourceURL: f.SourceUrl, SourceObservedAt: f.SourceObservedAt.UTC(), Geography: f.Geography, EvidenceID: idPtr(f.EvidenceID), CreatedAt: f.CreatedAt.UTC()}
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
