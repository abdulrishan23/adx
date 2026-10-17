package brands

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// FactClass is how far a fact may be trusted.
type FactClass string

const (
	// Approved: an admin signed it off. Only approved facts become claims.
	Approved FactClass = "approved"
	// NeedsReview: a person entered it and nobody has approved it yet, or it
	// is party to an open conflict.
	NeedsReview FactClass = "needs_review"
	// Inferred: ADX derived it from evidence. It is shown as such and is
	// never approved without a person.
	Inferred FactClass = "inferred"
	// Retired: no longer current. Kept for history with the reason.
	Retired FactClass = "retired"
)

type FactKind string

const (
	KindText    FactKind = "text"
	KindMoney   FactKind = "money"
	KindNumber  FactKind = "number"
	KindPercent FactKind = "percent"
	KindDate    FactKind = "date"
)

type FactSource string

const (
	// SourceManual: typed in by a person.
	SourceManual FactSource = "manual"
	// SourceDocument: read from a named document (a price list, a brochure).
	SourceDocument FactSource = "document"
	// SourceEvidence: derived from a stored observation; EvidenceID is set.
	SourceEvidence FactSource = "evidence"
)

// Fact is one assertion about a brand or one of its projects.
type Fact struct {
	ID            int64      `json:"id"`
	BrandID       int64      `json:"brandId"`
	ProjectID     *int64     `json:"projectId"`
	Key           string     `json:"key"`
	Kind          FactKind   `json:"kind"`
	Value         string     `json:"value"`
	Currency      *string    `json:"currency"`
	EffectiveFrom string     `json:"effectiveFrom"`
	EffectiveTo   *string    `json:"effectiveTo"`
	Class         FactClass  `json:"class"`
	SourceKind    FactSource `json:"sourceKind"`
	SourceRef     string     `json:"sourceRef"`
	EvidenceID    *int64     `json:"evidenceId"`
	Note          string     `json:"note"`
	CreatedBy     *int64     `json:"createdBy"`
	CreatedAt     time.Time  `json:"createdAt"`
	ApprovedBy    *int64     `json:"approvedBy"`
	ApprovedAt    *time.Time `json:"approvedAt"`
	RetiredAt     *time.Time `json:"retiredAt"`
	RetiredReason string     `json:"retiredReason"`
	SupersedesID  *int64     `json:"supersedesId"`
	// InConflict is true while an open conflict names this fact.
	InConflict bool `json:"inConflict"`
}

type FactInput struct {
	ProjectID     int64      `json:"projectId"`
	Key           string     `json:"key"`
	Kind          FactKind   `json:"kind"`
	Value         string     `json:"value"`
	Currency      string     `json:"currency"`
	EffectiveFrom string     `json:"effectiveFrom"`
	EffectiveTo   *string    `json:"effectiveTo"`
	SourceKind    FactSource `json:"sourceKind"`
	SourceRef     string     `json:"sourceRef"`
	// EvidenceID names the observation an inferred fact came from.
	EvidenceID int64  `json:"evidenceId"`
	Note       string `json:"note"`
	// SupersedesID names the fact this one corrects. The old fact is retired
	// in the same transaction with the note as the reason.
	SupersedesID int64 `json:"supersedesId"`
}

// Conflict is two current facts for the same key that disagree while their
// effective dates overlap. Open until an admin picks one; the other is
// retired with the resolution and both rows stay.
type Conflict struct {
	ID         int64      `json:"id"`
	BrandID    int64      `json:"brandId"`
	Key        string     `json:"key"`
	FactA      Fact       `json:"factA"`
	FactB      Fact       `json:"factB"`
	State      string     `json:"state"`
	OpenedAt   time.Time  `json:"openedAt"`
	ResolvedBy *int64     `json:"resolvedBy"`
	ResolvedAt *time.Time `json:"resolvedAt"`
	WinnerID   *int64     `json:"winnerId"`
	Resolution string     `json:"resolution"`
}

// FactChange is the answer to adding a fact: the fact and the conflicts it
// opened.
type FactChange struct {
	Fact      Fact       `json:"fact"`
	Conflicts []Conflict `json:"conflicts"`
}

// ClaimSet is what a draft may say about a project on a day: the approved
// facts effective then, most specific first. Facts awaiting review or in
// conflict are listed by key so the writer knows what is missing, with no
// value.
type ClaimSet struct {
	BrandID   int64    `json:"brandId"`
	ProjectID *int64   `json:"projectId"`
	At        string   `json:"at"`
	Claims    []Fact   `json:"claims"`
	Pending   []string `json:"pending"`
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)*$`)

// SuggestedKeys are the keys the UI offers. They are a vocabulary, not a
// rule: any key matching keyPattern is accepted.
var SuggestedKeys = []string{"price.from", "price.per_sqft", "payment_plan", "handover", "discount.max", "location", "unit_types", "service_charge", "developer", "rera_number"}

// AddFact records an assertion. People add facts as needs_review; the system
// adds inferred facts from evidence. Any current fact for the same key with
// a different value and overlapping dates opens a conflict, and the new fact
// is never approved on the way in.
func (b *Brands) AddFact(ctx context.Context, actor core.Actor, brandID int64, in FactInput) (FactChange, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return FactChange{}, core.ErrForbidden
	}
	if _, err := b.brand(ctx, actor.WorkspaceID, brandID); err != nil {
		return FactChange{}, err
	}
	in.Key = strings.ToLower(strings.TrimSpace(in.Key))
	if !keyPattern.MatchString(in.Key) || len(in.Key) > 80 {
		return FactChange{}, core.Invalid("key must look like price.from: lowercase words, digits, underscores and dots")
	}
	if in.ProjectID != 0 {
		owner, err := b.BrandOfProject(ctx, actor, in.ProjectID)
		if err != nil {
			return FactChange{}, err
		}
		if owner != brandID {
			return FactChange{}, core.Invalid("project %d does not belong to this brand", in.ProjectID)
		}
	}
	value, currency, err := normalizeValue(in.Kind, in.Value, in.Currency)
	if err != nil {
		return FactChange{}, err
	}
	from, to, err := effectiveWindow(in.EffectiveFrom, in.EffectiveTo)
	if err != nil {
		return FactChange{}, err
	}
	class := NeedsReview
	switch in.SourceKind {
	case SourceManual, SourceDocument:
		if in.EvidenceID != 0 {
			return FactChange{}, core.Invalid("only evidence-sourced facts name an evidence row")
		}
	case SourceEvidence:
		if in.EvidenceID == 0 {
			return FactChange{}, core.Invalid("an evidence-sourced fact needs the evidence id")
		}
		if actor.Via != core.ViaJob {
			return FactChange{}, core.Invalid("inferred facts are recorded by ADX from evidence; enter what you know as a manual or document fact")
		}
		class = Inferred
	default:
		return FactChange{}, core.Invalid("source must be manual, document or evidence")
	}
	now := b.clock.Now().UTC()
	var row store.Fact
	var opened []store.FactConflict
	err = b.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if in.SupersedesID != 0 {
			old, err := q.GetFact(ctx, store.GetFactParams{ID: in.SupersedesID, WorkspaceID: actor.WorkspaceID})
			if errors.Is(err, sql.ErrNoRows) {
				return core.NotFound("fact %d not found", in.SupersedesID)
			}
			if err != nil {
				return err
			}
			if old.BrandID != brandID || old.Key != in.Key {
				return core.Invalid("a fact can only supersede a fact for the same brand and key")
			}
			if FactClass(old.Class) != Retired {
				if err := q.RetireFact(ctx, store.RetireFactParams{RetiredAt: sql.NullTime{Time: now, Valid: true}, RetiredReason: "superseded: " + strings.TrimSpace(in.Note), ID: old.ID}); err != nil {
					return err
				}
			}
		}
		var err error
		row, err = q.CreateFact(ctx, store.CreateFactParams{
			WorkspaceID: actor.WorkspaceID, BrandID: brandID, ProjectID: optionalID(in.ProjectID), Key: in.Key, Kind: string(in.Kind), Value: value, Currency: currency,
			EffectiveFrom: from, EffectiveTo: to, Class: string(class), SourceKind: string(in.SourceKind), SourceRef: strings.TrimSpace(in.SourceRef), EvidenceID: optionalID(in.EvidenceID),
			Note: strings.TrimSpace(in.Note), CreatedBy: optionalID(actor.UserID), CreatedAt: now, SupersedesID: optionalID(in.SupersedesID),
		})
		if err != nil {
			return err
		}
		others, err := q.ListCurrentFactsByKey(ctx, store.ListCurrentFactsByKeyParams{WorkspaceID: actor.WorkspaceID, BrandID: brandID, Key: in.Key})
		if err != nil {
			return err
		}
		for _, o := range others {
			if o.ID == row.ID || !disagree(row, o) {
				continue
			}
			c, err := q.CreateFactConflict(ctx, store.CreateFactConflictParams{WorkspaceID: actor.WorkspaceID, BrandID: brandID, Key: in.Key, FactAID: o.ID, FactBID: row.ID, OpenedAt: now})
			if err != nil {
				return err
			}
			opened = append(opened, c)
		}
		return nil
	})
	if err != nil {
		return FactChange{}, err
	}
	fact := factFromRow(row)
	fact.InConflict = len(opened) > 0
	out := FactChange{Fact: fact, Conflicts: []Conflict{}}
	for _, c := range opened {
		cc, err := b.conflictFromRow(ctx, actor.WorkspaceID, c)
		if err != nil {
			return FactChange{}, err
		}
		out.Conflicts = append(out.Conflicts, cc)
	}
	return out, nil
}

// disagree is true when two current facts for one key can't both be claimed:
// same scope (the same project, or both brand-wide), different values and
// overlapping effective dates. A project fact beside a brand-wide one is
// not a conflict: the project's own wins for that project (Claims).
func disagree(a, o store.Fact) bool {
	if a.Value == o.Value && a.Currency.String == o.Currency.String {
		return false
	}
	if a.ProjectID != o.ProjectID {
		return false
	}
	return overlaps(a.EffectiveFrom, a.EffectiveTo, o.EffectiveFrom, o.EffectiveTo)
}

func overlaps(aFrom time.Time, aTo sql.NullTime, bFrom time.Time, bTo sql.NullTime) bool {
	if aTo.Valid && aTo.Time.Before(bFrom) {
		return false
	}
	if bTo.Valid && bTo.Time.Before(aFrom) {
		return false
	}
	return true
}

// ApproveFact signs a fact off. Admins only. A fact in an open conflict
// can't be approved: resolve the conflict first. An inferred fact becomes
// approved, which records that a person stood behind it.
func (b *Brands) ApproveFact(ctx context.Context, actor core.Actor, factID int64) (Fact, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return Fact{}, core.ErrForbidden
	}
	now := b.clock.Now().UTC()
	var row store.Fact
	err := b.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		f, err := q.GetFact(ctx, store.GetFactParams{ID: factID, WorkspaceID: actor.WorkspaceID})
		if errors.Is(err, sql.ErrNoRows) {
			return core.NotFound("fact %d not found", factID)
		}
		if err != nil {
			return err
		}
		if FactClass(f.Class) == Retired {
			return core.Conflict("fact %d is retired", factID)
		}
		open, err := q.ListOpenConflictsForFact(ctx, factID)
		if err != nil {
			return err
		}
		if len(open) > 0 {
			return core.Conflict("fact %d conflicts with another source; resolve the conflict first", factID)
		}
		if err := q.ApproveFact(ctx, store.ApproveFactParams{ApprovedBy: optionalID(actor.UserID), ApprovedAt: sql.NullTime{Time: now, Valid: true}, ID: factID}); err != nil {
			return err
		}
		row, err = q.GetFact(ctx, store.GetFactParams{ID: factID, WorkspaceID: actor.WorkspaceID})
		return err
	})
	if err != nil {
		return Fact{}, err
	}
	return factFromRow(row), nil
}

// RetireFact takes a fact out of use with a reason. Admins only. Open
// conflicts naming it close with the other fact as winner, still unapproved.
func (b *Brands) RetireFact(ctx context.Context, actor core.Actor, factID int64, reason string) (Fact, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return Fact{}, core.ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Fact{}, core.Invalid("a reason is required")
	}
	now := b.clock.Now().UTC()
	var row store.Fact
	err := b.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		f, err := q.GetFact(ctx, store.GetFactParams{ID: factID, WorkspaceID: actor.WorkspaceID})
		if errors.Is(err, sql.ErrNoRows) {
			return core.NotFound("fact %d not found", factID)
		}
		if err != nil {
			return err
		}
		if FactClass(f.Class) == Retired {
			row = f
			return nil
		}
		if err := q.RetireFact(ctx, store.RetireFactParams{RetiredAt: sql.NullTime{Time: now, Valid: true}, RetiredReason: reason, ID: factID}); err != nil {
			return err
		}
		open, err := q.ListOpenConflictsForFact(ctx, factID)
		if err != nil {
			return err
		}
		for _, c := range open {
			other := c.FactAID
			if other == factID {
				other = c.FactBID
			}
			if err := q.ResolveFactConflict(ctx, store.ResolveFactConflictParams{ResolvedBy: optionalID(actor.UserID), ResolvedAt: sql.NullTime{Time: now, Valid: true}, WinnerID: optionalID(other), Resolution: "other fact retired: " + reason, ID: c.ID}); err != nil {
				return err
			}
		}
		row, err = q.GetFact(ctx, store.GetFactParams{ID: factID, WorkspaceID: actor.WorkspaceID})
		return err
	})
	if err != nil {
		return Fact{}, err
	}
	return factFromRow(row), nil
}

// ResolveConflict picks the fact to keep. Admins only. The loser is retired
// with the resolution; the winner stays in its class (approve it separately
// if it should become a claim), so resolving never authorizes a claim by
// itself.
func (b *Brands) ResolveConflict(ctx context.Context, actor core.Actor, conflictID, winnerID int64, resolution string) (Conflict, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return Conflict{}, core.ErrForbidden
	}
	resolution = strings.TrimSpace(resolution)
	if resolution == "" {
		return Conflict{}, core.Invalid("say why this source wins")
	}
	now := b.clock.Now().UTC()
	var row store.FactConflict
	err := b.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		c, err := q.GetFactConflict(ctx, store.GetFactConflictParams{ID: conflictID, WorkspaceID: actor.WorkspaceID})
		if errors.Is(err, sql.ErrNoRows) {
			return core.NotFound("conflict %d not found", conflictID)
		}
		if err != nil {
			return err
		}
		if c.State != "open" {
			return core.Conflict("conflict %d is already resolved", conflictID)
		}
		loser := c.FactBID
		switch winnerID {
		case c.FactAID:
		case c.FactBID:
			loser = c.FactAID
		default:
			return core.Invalid("the winner must be one of the two facts in the conflict")
		}
		if err := q.RetireFact(ctx, store.RetireFactParams{RetiredAt: sql.NullTime{Time: now, Valid: true}, RetiredReason: "lost conflict: " + resolution, ID: loser}); err != nil {
			return err
		}
		if err := q.ResolveFactConflict(ctx, store.ResolveFactConflictParams{ResolvedBy: optionalID(actor.UserID), ResolvedAt: sql.NullTime{Time: now, Valid: true}, WinnerID: optionalID(winnerID), Resolution: resolution, ID: conflictID}); err != nil {
			return err
		}
		// The loser may have been in other conflicts; those close too.
		open, err := q.ListOpenConflictsForFact(ctx, loser)
		if err != nil {
			return err
		}
		for _, o := range open {
			other := o.FactAID
			if other == loser {
				other = o.FactBID
			}
			if err := q.ResolveFactConflict(ctx, store.ResolveFactConflictParams{ResolvedBy: optionalID(actor.UserID), ResolvedAt: sql.NullTime{Time: now, Valid: true}, WinnerID: optionalID(other), Resolution: "other fact retired: " + resolution, ID: o.ID}); err != nil {
				return err
			}
		}
		row, err = q.GetFactConflict(ctx, store.GetFactConflictParams{ID: conflictID, WorkspaceID: actor.WorkspaceID})
		return err
	})
	if err != nil {
		return Conflict{}, err
	}
	return b.conflictFromRow(ctx, actor.WorkspaceID, row)
}

// ListFacts is the brand's current (not retired) facts, optionally for one
// project plus the brand-wide ones.
func (b *Brands) ListFacts(ctx context.Context, actor core.Actor, brandID, projectID int64) ([]Fact, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := b.db.Reader().ListCurrentFacts(ctx, store.ListCurrentFactsParams{WorkspaceID: actor.WorkspaceID, BrandID: brandID, ProjectID: optionalID(projectID)})
	if err != nil {
		return nil, err
	}
	return b.factsWithConflicts(ctx, rows)
}

// FactHistory is every row ever written for a key, retired ones included.
func (b *Brands) FactHistory(ctx context.Context, actor core.Actor, brandID int64, key string) ([]Fact, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := b.db.Reader().ListFactHistory(ctx, store.ListFactHistoryParams{WorkspaceID: actor.WorkspaceID, BrandID: brandID, Key: strings.ToLower(strings.TrimSpace(key))})
	if err != nil {
		return nil, err
	}
	return b.factsWithConflicts(ctx, rows)
}

// ListConflicts is the brand's open conflicts.
func (b *Brands) ListConflicts(ctx context.Context, actor core.Actor, brandID int64) ([]Conflict, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := b.db.Reader().ListOpenConflicts(ctx, store.ListOpenConflictsParams{WorkspaceID: actor.WorkspaceID, BrandID: brandID})
	if err != nil {
		return nil, err
	}
	out := make([]Conflict, 0, len(rows))
	for _, r := range rows {
		c, err := b.conflictFromRow(ctx, actor.WorkspaceID, r)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// Claims is what may be said about a project on a day: approved facts in
// effect, the project's own over the brand's for the same key. Only this
// brand's facts are consulted; anything awaiting review is listed as pending
// without a value.
func (b *Brands) Claims(ctx context.Context, actor core.Actor, brandID, projectID int64, at time.Time) (ClaimSet, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ClaimSet{}, core.ErrForbidden
	}
	if _, err := b.brand(ctx, actor.WorkspaceID, brandID); err != nil {
		return ClaimSet{}, err
	}
	if projectID != 0 {
		owner, err := b.BrandOfProject(ctx, actor, projectID)
		if err != nil {
			return ClaimSet{}, err
		}
		if owner != brandID {
			return ClaimSet{}, core.Invalid("project %d does not belong to brand %d", projectID, brandID)
		}
	}
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	rows, err := b.db.Reader().ListClaims(ctx, store.ListClaimsParams{WorkspaceID: actor.WorkspaceID, BrandID: brandID, ProjectID: optionalID(projectID), At: day})
	if err != nil {
		return ClaimSet{}, err
	}
	out := ClaimSet{BrandID: brandID, ProjectID: idPtr(optionalID(projectID)), At: dateString(day), Claims: []Fact{}, Pending: []string{}}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.Key] {
			continue
		}
		seen[r.Key] = true
		out.Claims = append(out.Claims, factFromRow(r))
	}
	current, err := b.db.Reader().ListCurrentFacts(ctx, store.ListCurrentFactsParams{WorkspaceID: actor.WorkspaceID, BrandID: brandID, ProjectID: optionalID(projectID)})
	if err != nil {
		return ClaimSet{}, err
	}
	pending := map[string]bool{}
	for _, r := range current {
		if seen[r.Key] || pending[r.Key] || FactClass(r.Class) == Approved {
			continue
		}
		if !overlaps(r.EffectiveFrom, r.EffectiveTo, day, sql.NullTime{Time: day, Valid: true}) {
			continue
		}
		pending[r.Key] = true
		out.Pending = append(out.Pending, r.Key)
	}
	return out, nil
}

func (b *Brands) factsWithConflicts(ctx context.Context, rows []store.Fact) ([]Fact, error) {
	out := make([]Fact, 0, len(rows))
	for _, r := range rows {
		f := factFromRow(r)
		if f.Class != Retired {
			open, err := b.db.Reader().ListOpenConflictsForFact(ctx, r.ID)
			if err != nil {
				return nil, err
			}
			f.InConflict = len(open) > 0
		}
		out = append(out, f)
	}
	return out, nil
}

func (b *Brands) conflictFromRow(ctx context.Context, workspaceID int64, r store.FactConflict) (Conflict, error) {
	a, err := b.db.Reader().GetFact(ctx, store.GetFactParams{ID: r.FactAID, WorkspaceID: workspaceID})
	if err != nil {
		return Conflict{}, err
	}
	o, err := b.db.Reader().GetFact(ctx, store.GetFactParams{ID: r.FactBID, WorkspaceID: workspaceID})
	if err != nil {
		return Conflict{}, err
	}
	c := Conflict{ID: r.ID, BrandID: r.BrandID, Key: r.Key, FactA: factFromRow(a), FactB: factFromRow(o), State: r.State, OpenedAt: r.OpenedAt.UTC(), ResolvedBy: idPtr(r.ResolvedBy), WinnerID: idPtr(r.WinnerID), Resolution: r.Resolution}
	c.FactA.InConflict, c.FactB.InConflict = r.State == "open", r.State == "open"
	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time.UTC()
		c.ResolvedAt = &t
	}
	return c, nil
}

// normalizeValue checks a value against its kind and returns the stored
// form, so "1,200,000" and "1200000" compare equal.
func normalizeValue(kind FactKind, value, currency string) (string, sql.NullString, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", sql.NullString{}, core.Invalid("a value is required")
	}
	switch kind {
	case KindText:
		if len(value) > 500 {
			return "", sql.NullString{}, core.Invalid("text values are up to 500 characters")
		}
		return value, sql.NullString{}, nil
	case KindMoney:
		n, err := strconv.ParseInt(strings.ReplaceAll(value, ",", ""), 10, 64)
		if err != nil || n < 0 {
			return "", sql.NullString{}, core.Invalid("a money value is a whole number of minor units, such as 120000000 for AED 1,200,000")
		}
		currency = strings.ToUpper(strings.TrimSpace(currency))
		if len(currency) != 3 {
			return "", sql.NullString{}, core.Invalid("a money value needs a three-letter currency code")
		}
		return strconv.FormatInt(n, 10), sql.NullString{String: currency, Valid: true}, nil
	case KindNumber, KindPercent:
		f, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", ""), 64)
		if err != nil {
			return "", sql.NullString{}, core.Invalid("a %s value must be a number", kind)
		}
		if kind == KindPercent && (f < 0 || f > 100) {
			return "", sql.NullString{}, core.Invalid("a percent value is between 0 and 100")
		}
		return strconv.FormatFloat(f, 'f', -1, 64), sql.NullString{}, nil
	case KindDate:
		t, err := time.Parse(dateLayout, value)
		if err != nil {
			return "", sql.NullString{}, core.Invalid("a date value looks like 2027-06-30")
		}
		return t.Format(dateLayout), sql.NullString{}, nil
	}
	return "", sql.NullString{}, core.Invalid("kind must be text, money, number, percent or date")
}

func factFromRow(r store.Fact) Fact {
	f := Fact{ID: r.ID, BrandID: r.BrandID, ProjectID: idPtr(r.ProjectID), Key: r.Key, Kind: FactKind(r.Kind), Value: r.Value, EffectiveFrom: dateString(r.EffectiveFrom), EffectiveTo: dateStringPtr(r.EffectiveTo),
		Class: FactClass(r.Class), SourceKind: FactSource(r.SourceKind), SourceRef: r.SourceRef, EvidenceID: idPtr(r.EvidenceID), Note: r.Note, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(),
		ApprovedBy: idPtr(r.ApprovedBy), RetiredReason: r.RetiredReason, SupersedesID: idPtr(r.SupersedesID)}
	if r.Currency.Valid {
		c := r.Currency.String
		f.Currency = &c
	}
	if r.ApprovedAt.Valid {
		t := r.ApprovedAt.Time.UTC()
		f.ApprovedAt = &t
	}
	if r.RetiredAt.Valid {
		t := r.RetiredAt.Time.UTC()
		f.RetiredAt = &t
	}
	return f
}
