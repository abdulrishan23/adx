// Package crm keeps people apart from their enquiries. A person is who; an
// enquiry is a project they asked about; one person has as many enquiries
// as projects they looked at. Outside records (a CRM lead, a form
// submission) are linked to a person with the evidence that matched them.
// Matching is conservative: an email that only one person has is exact; a
// phone is exact only when one person has it and the name agrees; a phone
// several people share, or a name that disagrees, makes a new person and a
// visible candidate for someone to decide. Every link and merge can be
// reversed. What a customer stated is never overwritten by what a provider
// inferred; both are kept and a disagreement is shown.
package crm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/store"
)

type Person struct {
	ID           int64     `json:"id"`
	DisplayName  string    `json:"displayName"`
	PrimaryPhone *string   `json:"primaryPhone"`
	PrimaryEmail *string   `json:"primaryEmail"`
	Country      *string   `json:"country"`
	Language     *string   `json:"language"`
	Notes        string    `json:"notes"`
	MergedIntoID *int64    `json:"mergedIntoId"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Identifier struct {
	ID          int64     `json:"id"`
	Kind        string    `json:"kind"`
	Value       string    `json:"value"`
	Class       string    `json:"class"`
	Source      string    `json:"source"`
	EvidenceID  *int64    `json:"evidenceId"`
	FirstSeenAt time.Time `json:"firstSeenAt"`
}

type Enquiry struct {
	ID             int64     `json:"id"`
	PersonID       int64     `json:"personId"`
	ProjectID      *int64    `json:"projectId"`
	State          string    `json:"state"`
	Stage          *string   `json:"stage"`
	SourceKind     string    `json:"sourceKind"`
	SourceRef      string    `json:"sourceRef"`
	FirstContactAt time.Time `json:"firstContactAt"`
	LastActivityAt time.Time `json:"lastActivityAt"`
}

type Link struct {
	ID             int64           `json:"id"`
	PersonID       int64           `json:"personId"`
	Kind           string          `json:"kind"`
	Ref            string          `json:"ref"`
	MatchLevel     string          `json:"matchLevel"`
	MatchEvidence  json.RawMessage `json:"matchEvidence"`
	LinkedAt       time.Time       `json:"linkedAt"`
	LinkedBy       *int64          `json:"linkedBy"`
	ReversedAt     *time.Time      `json:"reversedAt"`
	ReversalReason string          `json:"reversalReason"`
}

type Candidate struct {
	ID          int64           `json:"id"`
	PersonID    int64           `json:"personId"`
	CandidateID int64           `json:"candidateId"`
	Reason      json.RawMessage `json:"reason"`
	State       string          `json:"state"`
	CreatedAt   time.Time       `json:"createdAt"`
	Other       *Person         `json:"other"`
}

type Attribute struct {
	ID         int64     `json:"id"`
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	Class      string    `json:"class"`
	Source     string    `json:"source"`
	EvidenceID *int64    `json:"evidenceId"`
	ObservedAt time.Time `json:"observedAt"`
}

// AttributeView is the effective value for a key: stated wins; an enriched
// value that disagrees is shown beside it, never over it.
type AttributeView struct {
	Key      string     `json:"key"`
	Value    string     `json:"value"`
	Class    string     `json:"class"`
	Enriched *Attribute `json:"enriched"`
	Conflict bool       `json:"conflict"`
}

type EnrichmentState struct {
	Configured bool     `json:"configured"`
	Provider   string   `json:"provider"`
	CostMinor  int64    `json:"costMinor"`
	Currency   string   `json:"currency"`
	Permitted  []string `json:"permitted"`
	Reason     string   `json:"reason"`
}

type PersonDetail struct {
	Person      Person          `json:"person"`
	Identifiers []Identifier    `json:"identifiers"`
	Enquiries   []Enquiry       `json:"enquiries"`
	Links       []Link          `json:"links"`
	Candidates  []Candidate     `json:"candidates"`
	Attributes  []AttributeView `json:"attributes"`
	Enrichment  EnrichmentState `json:"enrichment"`
}

type PersonSummary struct {
	Person
	Enquiries      int  `json:"enquiries"`
	OpenCandidates int  `json:"openCandidates"`
	Uncertain      bool `json:"uncertain"`
}

type PersonInput struct {
	DisplayName string `json:"displayName"`
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	Country     string `json:"country"`
	Language    string `json:"language"`
	Notes       string `json:"notes"`
}

// Enrichment provider contract. None is wired; a fake tests the path.

type Identity struct {
	Name  string
	Phone string
	Email string
}

type Enrichment struct {
	Attributes map[string]string
	// Freshness is when the provider last refreshed its data.
	Freshness time.Time
}

type Enricher interface {
	Name() string
	// Cost is what one lookup costs, for the record.
	Cost() (int64, string)
	// Permitted lists the attribute keys the provider may return.
	Permitted() []string
	Enrich(ctx context.Context, id Identity) (Enrichment, error)
}

type Config struct {
	Clock    core.Clock
	Enricher Enricher
	// PermittedAttributes is what ADX stores from any provider. Default:
	// company, job_title, country, language.
	PermittedAttributes []string
	// DefaultCountryCode is prefixed to national numbers. Default 971.
	DefaultCountryCode string
	// EnquiryWindow is how long a repeat enquiry for the same project stays
	// the same enquiry. Default 30 days.
	EnquiryWindow time.Duration
}

type CRM struct {
	db       *store.DB
	clock    core.Clock
	evidence *evidence.Store
	cfg      Config
}

func New(db *store.DB, ev *evidence.Store, cfg Config) *CRM {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if len(cfg.PermittedAttributes) == 0 {
		cfg.PermittedAttributes = []string{"company", "job_title", "country", "language"}
	}
	if cfg.DefaultCountryCode == "" {
		cfg.DefaultCountryCode = "971"
	}
	if cfg.EnquiryWindow <= 0 {
		cfg.EnquiryWindow = 30 * 24 * time.Hour
	}
	return &CRM{db: db, clock: cfg.Clock, evidence: ev, cfg: cfg}
}

// Normalization.

// NormalizePhone returns +<country><number> or "" when the digits can't be
// a phone. National UAE forms (05x..., 5x...) get the default country code.
func (c *CRM) NormalizePhone(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	d := b.String()
	d = strings.TrimPrefix(d, "00")
	switch {
	case len(d) == 10 && strings.HasPrefix(d, "0"):
		d = c.cfg.DefaultCountryCode + d[1:]
	case len(d) == 9 && strings.HasPrefix(d, "5"):
		d = c.cfg.DefaultCountryCode + d
	}
	if len(d) < 9 || len(d) > 15 {
		return ""
	}
	return "+" + d
}

func normalizeEmail(raw string) string {
	e := strings.ToLower(strings.TrimSpace(raw))
	if !strings.Contains(e, "@") || strings.ContainsAny(e, " \t") {
		return ""
	}
	return e
}

// namesAgree is a cautious first-name comparison: equal, or one a prefix of
// the other with at least three letters. Unknown names never agree.
func namesAgree(a, b string) bool {
	fa, fb := firstName(a), firstName(b)
	if fa == "" || fb == "" {
		return false
	}
	if fa == fb {
		return true
	}
	return len(fa) >= 3 && len(fb) >= 3 && (strings.HasPrefix(fa, fb) || strings.HasPrefix(fb, fa))
}

func firstName(s string) string {
	f := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	if len(f) == 0 {
		return ""
	}
	return strings.Trim(f[0], ".,")
}

// Sources.

// source is one outside record to resolve.
type source struct {
	kind, ref  string
	name       string
	phone      string
	email      string
	projectID  int64
	stage      *string
	at         time.Time
	evidenceID int64
	external   string
}

// Sync resolves every lead and non-test form submission not yet linked to a
// person. Marketers and jobs may. Idempotent: a linked record is skipped.
func (c *CRM) Sync(ctx context.Context, actor core.Actor) (int, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	n := 0
	leads, err := c.db.Reader().ListUnlinkedLeads(ctx, store.ListUnlinkedLeadsParams{WorkspaceID: actor.WorkspaceID, Limit: 500})
	if err != nil {
		return 0, err
	}
	for _, l := range leads {
		src := source{kind: "lead", ref: strconv.FormatInt(l.ID, 10), name: strings.TrimSpace(l.FirstName.String + " " + l.LastName.String), email: l.Email.String, projectID: l.ProjectID.Int64, at: l.FirstSeenAt, evidenceID: l.EvidenceID.Int64, external: "ghl:" + l.ExternalID}
		if l.PhoneE164.Valid && l.PhoneE164.String != "" {
			src.phone = l.PhoneE164.String
		} else {
			src.phone = l.Phone.String
		}
		if l.Stage.Valid {
			st := l.Stage.String
			src.stage = &st
		}
		if l.ProviderAddedAt.Valid {
			src.at = l.ProviderAddedAt.Time
		}
		if _, _, err := c.resolve(ctx, actor, src); err != nil {
			return n, fmt.Errorf("lead %d: %w", l.ID, err)
		}
		n++
	}
	subs, err := c.db.Reader().ListUnlinkedSubmissions(ctx, store.ListUnlinkedSubmissionsParams{WorkspaceID: actor.WorkspaceID, Limit: 500})
	if err != nil {
		return n, err
	}
	for _, s := range subs {
		var fields map[string]string
		_ = json.Unmarshal([]byte(s.Fields), &fields)
		src := source{kind: "form_submission", ref: strconv.FormatInt(s.ID, 10), projectID: s.ProjectID.Int64, at: s.ReceivedAt, evidenceID: s.EvidenceID.Int64}
		for k, v := range fields {
			lk := strings.ToLower(k)
			switch {
			case strings.Contains(lk, "phone") || strings.Contains(lk, "mobile"):
				if src.phone == "" {
					src.phone = v
				}
			case strings.Contains(lk, "email"):
				if src.email == "" {
					src.email = v
				}
			case lk == "name" || lk == "full_name" || lk == "fullname":
				src.name = v
			case lk == "first_name" || lk == "firstname":
				src.name = strings.TrimSpace(v + " " + fields["last_name"])
			}
		}
		if _, _, err := c.resolve(ctx, actor, src); err != nil {
			return n, fmt.Errorf("submission %d: %w", s.ID, err)
		}
		n++
	}
	return n, nil
}

// resolve links one record to a person, making one when nothing matches
// exactly, and records candidates for what matched only in part.
func (c *CRM) resolve(ctx context.Context, actor core.Actor, src source) (Person, string, error) {
	if existing, err := c.db.Reader().GetActiveLink(ctx, store.GetActiveLinkParams{WorkspaceID: actor.WorkspaceID, Kind: src.kind, Ref: src.ref}); err == nil {
		p, err := c.person(ctx, actor.WorkspaceID, existing.PersonID)
		return p, existing.MatchLevel, err
	}
	phone := c.NormalizePhone(src.phone)
	email := normalizeEmail(src.email)
	now := c.clock.Now().UTC()
	var target *store.Person
	level := "new"
	ev := map[string]any{}
	var candidates []store.Person
	if email != "" {
		byEmail, err := c.db.Reader().FindPeopleByIdentifier(ctx, store.FindPeopleByIdentifierParams{WorkspaceID: actor.WorkspaceID, Kind: "email", Value: email})
		if err != nil {
			return Person{}, "", err
		}
		if len(byEmail) == 1 {
			target, level, ev["email"] = &byEmail[0], "exact", true
		} else if len(byEmail) > 1 {
			candidates = append(candidates, byEmail...)
			ev["email"], ev["emailSharedBy"] = true, len(byEmail)
		}
	}
	if target == nil && phone != "" {
		byPhone, err := c.db.Reader().FindPeopleByIdentifier(ctx, store.FindPeopleByIdentifierParams{WorkspaceID: actor.WorkspaceID, Kind: "phone", Value: phone})
		if err != nil {
			return Person{}, "", err
		}
		// A phone is exact only when it points at one person whose name
		// agrees. A family phone shared by several people is exact for the
		// one whose name agrees, and a candidate match for everyone otherwise.
		var agreeing []store.Person
		for _, p := range byPhone {
			if namesAgree(p.DisplayName, src.name) {
				agreeing = append(agreeing, p)
			}
		}
		switch {
		case len(agreeing) == 1:
			target, level, ev["phone"], ev["name"] = &agreeing[0], "exact", true, "agrees"
			if len(byPhone) > 1 {
				ev["phoneSharedBy"] = len(byPhone)
			}
		case len(byPhone) > 0:
			candidates = append(candidates, byPhone...)
			ev["phone"], ev["phoneSharedBy"] = true, len(byPhone)
			if len(agreeing) == 0 {
				ev["name"] = "differs or unknown"
			} else {
				ev["name"] = "agrees with several"
			}
		}
	}
	var prow store.Person
	err := c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		if target != nil {
			prow = *target
			name := prow.DisplayName
			if name == "" {
				name = strings.TrimSpace(src.name)
			}
			pp, pe := prow.PrimaryPhone, prow.PrimaryEmail
			if !pp.Valid && phone != "" {
				pp = sql.NullString{String: phone, Valid: true}
			}
			if !pe.Valid && email != "" {
				pe = sql.NullString{String: email, Valid: true}
			}
			if err := q.UpdatePersonPrimary(ctx, store.UpdatePersonPrimaryParams{DisplayName: name, PrimaryPhone: pp, PrimaryEmail: pe, UpdatedAt: now, ID: prow.ID}); err != nil {
				return err
			}
		} else {
			prow, err = q.CreatePerson(ctx, store.CreatePersonParams{WorkspaceID: actor.WorkspaceID, DisplayName: strings.TrimSpace(src.name), PrimaryPhone: nullStr(phone), PrimaryEmail: nullStr(email), CreatedAt: now, UpdatedAt: now})
			if err != nil {
				return err
			}
			for _, cand := range candidates {
				reason, _ := json.Marshal(ev)
				if err := q.CreateMatchCandidate(ctx, store.CreateMatchCandidateParams{WorkspaceID: actor.WorkspaceID, PersonID: prow.ID, CandidateID: cand.ID, Reason: string(reason), CreatedAt: now}); err != nil {
					return err
				}
			}
		}
		for kind, value := range map[string]string{"phone": phone, "email": email, "external": src.external} {
			if value == "" {
				continue
			}
			if err := q.CreatePersonIdentifier(ctx, store.CreatePersonIdentifierParams{WorkspaceID: actor.WorkspaceID, PersonID: prow.ID, Kind: kind, Value: value, Class: "stated", Source: src.kind, EvidenceID: optionalID(src.evidenceID), FirstSeenAt: src.at}); err != nil {
				return err
			}
		}
		evJSON, _ := json.Marshal(ev)
		if _, err := q.CreatePersonLink(ctx, store.CreatePersonLinkParams{WorkspaceID: actor.WorkspaceID, PersonID: prow.ID, Kind: src.kind, Ref: src.ref, MatchLevel: level, MatchEvidence: string(evJSON), LinkedAt: now, LinkedBy: optionalID(actor.UserID)}); err != nil {
			return err
		}
		open, err := q.FindOpenEnquiry(ctx, store.FindOpenEnquiryParams{PersonID: prow.ID, ProjectID: optionalID(src.projectID)})
		switch {
		case err == nil && src.at.Sub(open.LastActivityAt) <= c.cfg.EnquiryWindow && src.at.After(open.FirstContactAt.Add(-c.cfg.EnquiryWindow)):
			return q.TouchEnquiry(ctx, store.TouchEnquiryParams{LastActivityAt: maxTime(open.LastActivityAt, src.at), Stage: nullStrPtr(src.stage), ID: open.ID})
		case err == nil || errors.Is(err, sql.ErrNoRows):
			_, err = q.CreateEnquiry(ctx, store.CreateEnquiryParams{WorkspaceID: actor.WorkspaceID, PersonID: prow.ID, ProjectID: optionalID(src.projectID), Stage: nullStrPtr(src.stage), SourceKind: src.kind, SourceRef: src.ref, FirstContactAt: src.at, LastActivityAt: src.at, CreatedAt: now})
			return err
		default:
			return err
		}
	})
	if err != nil {
		return Person{}, "", err
	}
	p, err := c.person(ctx, actor.WorkspaceID, prow.ID)
	return p, level, err
}

// People.

func (c *CRM) Create(ctx context.Context, actor core.Actor, in PersonInput) (PersonDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return PersonDetail{}, core.ErrForbidden
	}
	phone, email := c.NormalizePhone(in.Phone), normalizeEmail(in.Email)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.DisplayName == "" && phone == "" && email == "" {
		return PersonDetail{}, core.Invalid("a name, phone or email is required")
	}
	if strings.TrimSpace(in.Phone) != "" && phone == "" {
		return PersonDetail{}, core.Invalid("the phone number could not be read")
	}
	if strings.TrimSpace(in.Email) != "" && email == "" {
		return PersonDetail{}, core.Invalid("the email address could not be read")
	}
	now := c.clock.Now().UTC()
	var prow store.Person
	err := c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		prow, err = q.CreatePerson(ctx, store.CreatePersonParams{WorkspaceID: actor.WorkspaceID, DisplayName: in.DisplayName, PrimaryPhone: nullStr(phone), PrimaryEmail: nullStr(email), Country: nullStr(strings.ToUpper(strings.TrimSpace(in.Country))), Language: nullStr(strings.ToLower(strings.TrimSpace(in.Language))), Notes: strings.TrimSpace(in.Notes), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		for kind, value := range map[string]string{"phone": phone, "email": email} {
			if value != "" {
				if err := q.CreatePersonIdentifier(ctx, store.CreatePersonIdentifierParams{WorkspaceID: actor.WorkspaceID, PersonID: prow.ID, Kind: kind, Value: value, Class: "stated", Source: "manual", FirstSeenAt: now}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return PersonDetail{}, err
	}
	return c.Get(ctx, actor, prow.ID)
}

func (c *CRM) List(ctx context.Context, actor core.Actor, q string, limit int) ([]PersonSummary, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := c.db.Reader().ListPeople(ctx, store.ListPeopleParams{WorkspaceID: actor.WorkspaceID, Q: nullStr(strings.TrimSpace(q)), Max: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]PersonSummary, 0, len(rows))
	for _, r := range rows {
		enq, err := c.db.Reader().ListEnquiriesForPerson(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		cands, err := c.db.Reader().ListOpenCandidatesForPerson(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, PersonSummary{Person: personFromRow(r), Enquiries: len(enq), OpenCandidates: len(cands), Uncertain: len(cands) > 0})
	}
	return out, nil
}

func (c *CRM) Get(ctx context.Context, actor core.Actor, id int64) (PersonDetail, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return PersonDetail{}, core.ErrForbidden
	}
	prow, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return PersonDetail{}, core.NotFound("person %d not found", id)
	}
	if err != nil {
		return PersonDetail{}, err
	}
	d := PersonDetail{Person: personFromRow(prow), Identifiers: []Identifier{}, Enquiries: []Enquiry{}, Links: []Link{}, Candidates: []Candidate{}, Attributes: []AttributeView{}, Enrichment: c.EnrichmentState()}
	ids, err := c.db.Reader().ListPersonIdentifiers(ctx, id)
	if err != nil {
		return PersonDetail{}, err
	}
	for _, r := range ids {
		d.Identifiers = append(d.Identifiers, Identifier{ID: r.ID, Kind: r.Kind, Value: r.Value, Class: r.Class, Source: r.Source, EvidenceID: idPtr(r.EvidenceID), FirstSeenAt: r.FirstSeenAt.UTC()})
	}
	enq, err := c.db.Reader().ListEnquiriesForPerson(ctx, id)
	if err != nil {
		return PersonDetail{}, err
	}
	for _, r := range enq {
		d.Enquiries = append(d.Enquiries, enquiryFromRow(r))
	}
	links, err := c.db.Reader().ListPersonLinks(ctx, id)
	if err != nil {
		return PersonDetail{}, err
	}
	for _, r := range links {
		d.Links = append(d.Links, linkFromRow(r))
	}
	cands, err := c.db.Reader().ListOpenCandidatesForPerson(ctx, id)
	if err != nil {
		return PersonDetail{}, err
	}
	for _, r := range cands {
		cd := Candidate{ID: r.ID, PersonID: r.PersonID, CandidateID: r.CandidateID, Reason: json.RawMessage(r.Reason), State: r.State, CreatedAt: r.CreatedAt.UTC()}
		other := r.CandidateID
		if other == id {
			other = r.PersonID
		}
		if op, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: other, WorkspaceID: actor.WorkspaceID}); err == nil {
			o := personFromRow(op)
			cd.Other = &o
		}
		d.Candidates = append(d.Candidates, cd)
	}
	attrs, err := c.db.Reader().ListPersonAttributes(ctx, id)
	if err != nil {
		return PersonDetail{}, err
	}
	d.Attributes = effective(attrs)
	return d, nil
}

func effective(rows []store.PersonAttribute) []AttributeView {
	byKey := map[string]*AttributeView{}
	order := []string{}
	for _, r := range rows {
		a := Attribute{ID: r.ID, Key: r.Key, Value: r.Value, Class: r.Class, Source: r.Source, EvidenceID: idPtr(r.EvidenceID), ObservedAt: r.ObservedAt.UTC()}
		v, ok := byKey[r.Key]
		if !ok {
			v = &AttributeView{Key: r.Key}
			byKey[r.Key] = v
			order = append(order, r.Key)
		}
		switch r.Class {
		case "stated":
			v.Value, v.Class = a.Value, "stated"
		case "enriched":
			e := a
			v.Enriched = &e
			if v.Class == "" {
				v.Value, v.Class = a.Value, "enriched"
			}
		}
	}
	out := []AttributeView{}
	sort.Strings(order)
	for _, k := range order {
		v := byKey[k]
		if v.Class == "stated" && v.Enriched != nil && !strings.EqualFold(strings.TrimSpace(v.Enriched.Value), strings.TrimSpace(v.Value)) {
			v.Conflict = true
		}
		out = append(out, *v)
	}
	return out
}

// Corrections.

// ReverseLink detaches a record from its person. Marketers may. The record
// becomes unlinked and the next Sync resolves it again (or a person links
// it by hand).
func (c *CRM) ReverseLink(ctx context.Context, actor core.Actor, linkID int64, reason string) (Link, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 {
		return Link{}, core.ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Link{}, core.Invalid("a reason is required")
	}
	if _, err := c.db.Reader().GetPersonLink(ctx, store.GetPersonLinkParams{ID: linkID, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return Link{}, core.NotFound("link %d not found", linkID)
	} else if err != nil {
		return Link{}, err
	}
	now := c.clock.Now().UTC()
	n, err := c.db.Writer().ReversePersonLink(ctx, store.ReversePersonLinkParams{ReversedAt: sql.NullTime{Time: now, Valid: true}, ReversedBy: optionalID(actor.UserID), ReversalReason: reason, ID: linkID})
	if err != nil {
		return Link{}, err
	}
	if n != 1 {
		return Link{}, core.Conflict("link %d is already reversed", linkID)
	}
	row, err := c.db.Reader().GetPersonLink(ctx, store.GetPersonLinkParams{ID: linkID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return Link{}, err
	}
	return linkFromRow(row), nil
}

// LinkManually ties an unlinked record to a chosen person. Marketers may.
func (c *CRM) LinkManually(ctx context.Context, actor core.Actor, personID int64, kind, ref string) (Link, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 {
		return Link{}, core.ErrForbidden
	}
	if kind != "lead" && kind != "form_submission" {
		return Link{}, core.Invalid("kind must be lead or form_submission")
	}
	if _, err := c.person(ctx, actor.WorkspaceID, personID); err != nil {
		return Link{}, err
	}
	refID, _ := strconv.ParseInt(ref, 10, 64)
	switch kind {
	case "lead":
		if _, err := c.db.Reader().GetLead(ctx, store.GetLeadParams{ID: refID, WorkspaceID: actor.WorkspaceID}); err != nil {
			return Link{}, core.NotFound("lead %s not found", ref)
		}
	case "form_submission":
		if _, err := c.db.Reader().GetFormSubmission(ctx, store.GetFormSubmissionParams{ID: refID, WorkspaceID: actor.WorkspaceID}); err != nil {
			return Link{}, core.NotFound("submission %s not found", ref)
		}
	}
	if _, err := c.db.Reader().GetActiveLink(ctx, store.GetActiveLinkParams{WorkspaceID: actor.WorkspaceID, Kind: kind, Ref: ref}); err == nil {
		return Link{}, core.Conflict("%s %s is already linked; reverse that link first", kind, ref)
	}
	now := c.clock.Now().UTC()
	evJSON, _ := json.Marshal(map[string]any{"manual": true, "by": actor.UserID})
	row, err := c.db.Writer().CreatePersonLink(ctx, store.CreatePersonLinkParams{WorkspaceID: actor.WorkspaceID, PersonID: personID, Kind: kind, Ref: ref, MatchLevel: "manual", MatchEvidence: string(evJSON), LinkedAt: now, LinkedBy: optionalID(actor.UserID)})
	if err != nil {
		return Link{}, err
	}
	return linkFromRow(row), nil
}

// Decide settles a candidate. Accepting merges the newer person into the
// candidate; rejecting leaves both as they are. Marketers may.
func (c *CRM) Decide(ctx context.Context, actor core.Actor, candidateID int64, accept bool, note string) (PersonDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 {
		return PersonDetail{}, core.ErrForbidden
	}
	row, err := c.db.Reader().GetMatchCandidate(ctx, store.GetMatchCandidateParams{ID: candidateID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return PersonDetail{}, core.NotFound("candidate %d not found", candidateID)
	}
	if err != nil {
		return PersonDetail{}, err
	}
	state := "rejected"
	if accept {
		state = "accepted"
	}
	now := c.clock.Now().UTC()
	n, err := c.db.Writer().DecideMatchCandidate(ctx, store.DecideMatchCandidateParams{State: state, DecidedBy: optionalID(actor.UserID), DecidedAt: sql.NullTime{Time: now, Valid: true}, Note: strings.TrimSpace(note), ID: candidateID})
	if err != nil {
		return PersonDetail{}, err
	}
	if n != 1 {
		return PersonDetail{}, core.Conflict("candidate %d is already decided", candidateID)
	}
	if !accept {
		return c.Get(ctx, actor, row.PersonID)
	}
	return c.Merge(ctx, actor, row.PersonID, row.CandidateID, "accepted match candidate "+strconv.FormatInt(candidateID, 10)+": "+strings.TrimSpace(note))
}

type mergeRecord struct {
	Into        int64                    `json:"into"`
	Reason      string                   `json:"reason"`
	At          time.Time                `json:"at"`
	By          int64                    `json:"by"`
	Identifiers []store.PersonIdentifier `json:"identifiers"`
	LinkIDs     []int64                  `json:"linkIds"`
	EnquiryIDs  []int64                  `json:"enquiryIds"`
	AttrIDs     []int64                  `json:"attrIds"`
}

// Merge moves everything from one person into another and records what
// moved, so Unmerge can put it back. Marketers may.
func (c *CRM) Merge(ctx context.Context, actor core.Actor, fromID, intoID int64, reason string) (PersonDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 {
		return PersonDetail{}, core.ErrForbidden
	}
	if fromID == intoID {
		return PersonDetail{}, core.Invalid("a person can't be merged into themselves")
	}
	from, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: fromID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return PersonDetail{}, core.NotFound("person %d not found", fromID)
	}
	if from.MergedIntoID.Valid {
		return PersonDetail{}, core.Conflict("person %d is already merged", fromID)
	}
	into, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: intoID, WorkspaceID: actor.WorkspaceID})
	if err != nil || into.MergedIntoID.Valid {
		return PersonDetail{}, core.NotFound("person %d not found or merged", intoID)
	}
	now := c.clock.Now().UTC()
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		rec := mergeRecord{Into: intoID, Reason: strings.TrimSpace(reason), At: now, By: actor.UserID}
		ids, err := q.ListPersonIdentifiers(ctx, fromID)
		if err != nil {
			return err
		}
		rec.Identifiers = ids
		for _, i := range ids {
			if err := q.CreatePersonIdentifier(ctx, store.CreatePersonIdentifierParams{WorkspaceID: i.WorkspaceID, PersonID: intoID, Kind: i.Kind, Value: i.Value, Class: i.Class, Source: i.Source, EvidenceID: i.EvidenceID, FirstSeenAt: i.FirstSeenAt}); err != nil {
				return err
			}
		}
		if err := q.DeletePersonIdentifiers(ctx, fromID); err != nil {
			return err
		}
		links, err := q.ListPersonLinks(ctx, fromID)
		if err != nil {
			return err
		}
		for _, l := range links {
			if !l.ReversedAt.Valid {
				rec.LinkIDs = append(rec.LinkIDs, l.ID)
			}
		}
		if err := q.MovePersonLinks(ctx, store.MovePersonLinksParams{PersonID: intoID, PersonID_2: fromID}); err != nil {
			return err
		}
		enqs, err := q.ListEnquiriesForPerson(ctx, fromID)
		if err != nil {
			return err
		}
		for _, e := range enqs {
			rec.EnquiryIDs = append(rec.EnquiryIDs, e.ID)
		}
		if err := q.MoveEnquiries(ctx, store.MoveEnquiriesParams{PersonID: intoID, PersonID_2: fromID}); err != nil {
			return err
		}
		attrs, err := q.ListAllPersonAttributes(ctx, fromID)
		if err != nil {
			return err
		}
		for _, a := range attrs {
			rec.AttrIDs = append(rec.AttrIDs, a.ID)
		}
		if err := q.MovePersonAttributes(ctx, store.MovePersonAttributesParams{PersonID: intoID, PersonID_2: fromID}); err != nil {
			return err
		}
		raw, _ := json.Marshal(rec)
		if err := q.SetPersonMerged(ctx, store.SetPersonMergedParams{MergedIntoID: sql.NullInt64{Int64: intoID, Valid: true}, MergeRecord: string(raw), UpdatedAt: now, ID: fromID}); err != nil {
			return err
		}
		name := into.DisplayName
		if name == "" {
			name = from.DisplayName
		}
		pp, pe := into.PrimaryPhone, into.PrimaryEmail
		if !pp.Valid {
			pp = from.PrimaryPhone
		}
		if !pe.Valid {
			pe = from.PrimaryEmail
		}
		return q.UpdatePersonPrimary(ctx, store.UpdatePersonPrimaryParams{DisplayName: name, PrimaryPhone: pp, PrimaryEmail: pe, UpdatedAt: now, ID: intoID})
	})
	if err != nil {
		return PersonDetail{}, err
	}
	return c.Get(ctx, actor, intoID)
}

// Unmerge reverses a merge using the record it kept. Marketers may.
func (c *CRM) Unmerge(ctx context.Context, actor core.Actor, personID int64, reason string) (PersonDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 {
		return PersonDetail{}, core.ErrForbidden
	}
	row, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: personID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return PersonDetail{}, core.NotFound("person %d not found", personID)
	}
	if !row.MergedIntoID.Valid {
		return PersonDetail{}, core.Conflict("person %d is not merged", personID)
	}
	var rec mergeRecord
	if err := json.Unmarshal([]byte(row.MergeRecord), &rec); err != nil {
		return PersonDetail{}, core.Conflict("the merge record is unreadable; undo by hand")
	}
	now := c.clock.Now().UTC()
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		for _, i := range rec.Identifiers {
			if err := q.CreatePersonIdentifier(ctx, store.CreatePersonIdentifierParams{WorkspaceID: i.WorkspaceID, PersonID: personID, Kind: i.Kind, Value: i.Value, Class: i.Class, Source: i.Source, EvidenceID: i.EvidenceID, FirstSeenAt: i.FirstSeenAt}); err != nil {
				return err
			}
		}
		for _, id := range rec.LinkIDs {
			if err := q.SetPersonLinkPerson(ctx, store.SetPersonLinkPersonParams{PersonID: personID, ID: id}); err != nil {
				return err
			}
		}
		for _, id := range rec.EnquiryIDs {
			if err := q.SetEnquiryPerson(ctx, store.SetEnquiryPersonParams{PersonID: personID, ID: id}); err != nil {
				return err
			}
		}
		for _, id := range rec.AttrIDs {
			if err := q.SetAttributePerson(ctx, store.SetAttributePersonParams{PersonID: personID, ID: id}); err != nil {
				return err
			}
		}
		undone, _ := json.Marshal(map[string]any{"undoneAt": now, "undoneBy": actor.UserID, "reason": strings.TrimSpace(reason), "was": rec})
		return q.SetPersonMerged(ctx, store.SetPersonMergedParams{MergedIntoID: sql.NullInt64{}, MergeRecord: string(undone), UpdatedAt: now, ID: personID})
	})
	if err != nil {
		return PersonDetail{}, err
	}
	return c.Get(ctx, actor, personID)
}

// Attributes and enrichment.

// SetStated records what the customer said. Marketers may. It supersedes an
// earlier stated value for the key; an enriched value is left alone.
func (c *CRM) SetStated(ctx context.Context, actor core.Actor, personID int64, key, value, source string) (PersonDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return PersonDetail{}, core.ErrForbidden
	}
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return PersonDetail{}, core.Invalid("a key and a value are required")
	}
	if _, err := c.person(ctx, actor.WorkspaceID, personID); err != nil {
		return PersonDetail{}, err
	}
	now := c.clock.Now().UTC()
	if source == "" {
		source = "manual"
	}
	err := c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if err := q.SupersedePersonAttribute(ctx, store.SupersedePersonAttributeParams{SupersededAt: sql.NullTime{Time: now, Valid: true}, PersonID: personID, Key: key, Class: "stated"}); err != nil {
			return err
		}
		_, err := q.CreatePersonAttribute(ctx, store.CreatePersonAttributeParams{WorkspaceID: actor.WorkspaceID, PersonID: personID, Key: key, Value: value, Class: "stated", Source: source, ObservedAt: now})
		return err
	})
	if err != nil {
		return PersonDetail{}, err
	}
	return c.Get(ctx, actor, personID)
}

func (c *CRM) EnrichmentState() EnrichmentState {
	s := EnrichmentState{Permitted: c.cfg.PermittedAttributes}
	if c.cfg.Enricher == nil {
		s.Reason = "no enrichment provider is configured; nothing is looked up about anyone"
		return s
	}
	s.Configured, s.Provider = true, c.cfg.Enricher.Name()
	s.CostMinor, s.Currency = c.cfg.Enricher.Cost()
	return s
}

// Enrich asks the provider about a person and stores only permitted
// attributes, as enriched, beside anything the customer stated. Marketers
// may. Each lookup is evidence with its cost and freshness.
func (c *CRM) Enrich(ctx context.Context, actor core.Actor, personID int64) (PersonDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return PersonDetail{}, core.ErrForbidden
	}
	if c.cfg.Enricher == nil {
		return PersonDetail{}, core.Conflict("no enrichment provider is configured on this server")
	}
	p, err := c.person(ctx, actor.WorkspaceID, personID)
	if err != nil {
		return PersonDetail{}, err
	}
	id := Identity{Name: p.DisplayName}
	if p.PrimaryPhone != nil {
		id.Phone = *p.PrimaryPhone
	}
	if p.PrimaryEmail != nil {
		id.Email = *p.PrimaryEmail
	}
	res, err := c.cfg.Enricher.Enrich(ctx, id)
	if err != nil {
		return PersonDetail{}, core.Unavailable(err, "the enrichment provider did not answer")
	}
	permitted := map[string]bool{}
	for _, k := range c.cfg.PermittedAttributes {
		permitted[k] = true
	}
	providerAllows := map[string]bool{}
	for _, k := range c.cfg.Enricher.Permitted() {
		providerAllows[k] = true
	}
	kept, dropped := map[string]string{}, []string{}
	for k, v := range res.Attributes {
		k = strings.ToLower(strings.TrimSpace(k))
		if permitted[k] && providerAllows[k] && strings.TrimSpace(v) != "" {
			kept[k] = strings.TrimSpace(v)
		} else {
			dropped = append(dropped, k)
		}
	}
	sort.Strings(dropped)
	cost, cur := c.cfg.Enricher.Cost()
	now := c.clock.Now().UTC()
	var evidenceID sql.NullInt64
	if c.evidence != nil {
		ev, _, err := c.evidence.Record(ctx, actor, evidence.Observation{Source: "enrichment", Kind: c.cfg.Enricher.Name(), SourceRef: fmt.Sprintf("person:%d/%s", personID, now.Format(time.RFC3339Nano)), Scope: fmt.Sprintf("person:%d", personID), Class: evidence.Inferred,
			Payload: map[string]any{"kept": kept, "droppedKeys": dropped, "costMinor": cost, "currency": cur, "providerFreshness": res.Freshness}, Completeness: evidence.Complete})
		if err != nil {
			return PersonDetail{}, err
		}
		evidenceID = sql.NullInt64{Int64: ev.ID, Valid: true}
	}
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		for k, v := range kept {
			if err := q.SupersedePersonAttribute(ctx, store.SupersedePersonAttributeParams{SupersededAt: sql.NullTime{Time: now, Valid: true}, PersonID: personID, Key: k, Class: "enriched"}); err != nil {
				return err
			}
			if _, err := q.CreatePersonAttribute(ctx, store.CreatePersonAttributeParams{WorkspaceID: actor.WorkspaceID, PersonID: personID, Key: k, Value: v, Class: "enriched", Source: c.cfg.Enricher.Name(), EvidenceID: evidenceID, ObservedAt: res.Freshness.UTC()}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return PersonDetail{}, err
	}
	return c.Get(ctx, actor, personID)
}

// Enquiries and candidates lists.

func (c *CRM) ListEnquiries(ctx context.Context, actor core.Actor, projectID int64, limit int) ([]Enquiry, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := c.db.Reader().ListEnquiries(ctx, store.ListEnquiriesParams{WorkspaceID: actor.WorkspaceID, ProjectID: optionalID(projectID), Max: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Enquiry, 0, len(rows))
	for _, r := range rows {
		out = append(out, enquiryFromRow(r))
	}
	return out, nil
}

func (c *CRM) OpenCandidates(ctx context.Context, actor core.Actor, limit int) ([]Candidate, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := c.db.Reader().ListOpenCandidates(ctx, store.ListOpenCandidatesParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(rows))
	for _, r := range rows {
		cd := Candidate{ID: r.ID, PersonID: r.PersonID, CandidateID: r.CandidateID, Reason: json.RawMessage(r.Reason), State: r.State, CreatedAt: r.CreatedAt.UTC()}
		if op, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: r.CandidateID, WorkspaceID: actor.WorkspaceID}); err == nil {
			o := personFromRow(op)
			cd.Other = &o
		}
		out = append(out, cd)
	}
	return out, nil
}

func (c *CRM) person(ctx context.Context, workspaceID, id int64) (Person, error) {
	row, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Person{}, core.NotFound("person %d not found", id)
	}
	if err != nil {
		return Person{}, err
	}
	return personFromRow(row), nil
}

func personFromRow(r store.Person) Person {
	return Person{ID: r.ID, DisplayName: r.DisplayName, PrimaryPhone: strPtr(r.PrimaryPhone), PrimaryEmail: strPtr(r.PrimaryEmail), Country: strPtr(r.Country), Language: strPtr(r.Language), Notes: r.Notes, MergedIntoID: idPtr(r.MergedIntoID), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func enquiryFromRow(r store.Enquiry) Enquiry {
	return Enquiry{ID: r.ID, PersonID: r.PersonID, ProjectID: idPtr(r.ProjectID), State: r.State, Stage: strPtr(r.Stage), SourceKind: r.SourceKind, SourceRef: r.SourceRef, FirstContactAt: r.FirstContactAt.UTC(), LastActivityAt: r.LastActivityAt.UTC()}
}

func linkFromRow(r store.PersonLink) Link {
	l := Link{ID: r.ID, PersonID: r.PersonID, Kind: r.Kind, Ref: r.Ref, MatchLevel: r.MatchLevel, MatchEvidence: json.RawMessage(r.MatchEvidence), LinkedAt: r.LinkedAt.UTC(), LinkedBy: idPtr(r.LinkedBy), ReversalReason: r.ReversalReason}
	if r.ReversedAt.Valid {
		t := r.ReversedAt.Time.UTC()
		l.ReversedAt = &t
	}
	return l
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func nullStrPtr(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func strPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
