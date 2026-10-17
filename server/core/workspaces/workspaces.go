// Package workspaces provisions clients. A workspace is one client's whole
// world: its accounts, projects, people, plans and history. Rows carry a
// workspace id and every read filters by the actor's, so the same provider
// id or project name in two workspaces never meets. Industry configuration
// is versioned per workspace so one client's definitions never change under
// another's history.
package workspaces

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

type Workspaces struct {
	db  *store.DB
	now func() time.Time
}

func New(db *store.DB, now func() time.Time) *Workspaces {
	if now == nil {
		now = time.Now
	}
	return &Workspaces{db: db, now: now}
}

// Workspace is a client as the API shows it. Role is the caller's role in it.
type Workspace struct {
	ID        int64     `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Role      core.Role `json:"role"`
	Current   bool      `json:"current"`
	CreatedAt time.Time `json:"createdAt"`
}

type WorkspaceInput struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Industry seeds the first industry configuration. Empty leaves the
	// workspace without one until an admin writes it.
	Industry string `json:"industry"`
}

// Member is a person's membership in the current workspace.
type Member struct {
	UserID  int64     `json:"userId"`
	Email   string    `json:"email"`
	Name    *string   `json:"name"`
	Role    core.Role `json:"role"`
	Enabled bool      `json:"enabled"`
}

type MemberInput struct {
	Email string    `json:"email"`
	Role  core.Role `json:"role"`
}

// IndustryDocument is what a client's business looks like to ADX: the
// vocabulary its people use, the stages an enquiry moves through, what
// qualifies a buyer. It configures words and thresholds, never permissions:
// a document naming roles or permissions is refused, because configuration
// must not widen what anyone may do.
type IndustryDocument struct {
	Industry   string            `json:"industry"`
	Vocabulary map[string]string `json:"vocabulary"`
	Stages     []string          `json:"stages"`
	Qualifiers []string          `json:"qualifiers"`
	Notes      string            `json:"notes"`
}

// IndustryConfig is one version of a workspace's industry configuration.
type IndustryConfig struct {
	Version   int64            `json:"version"`
	Document  IndustryDocument `json:"document"`
	CreatedBy *int64           `json:"createdBy"`
	CreatedAt time.Time        `json:"createdAt"`
}

// IndustryState is the current configuration or its absence.
type IndustryState struct {
	Current  *IndustryConfig `json:"current"`
	Versions []int64         `json:"versions"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Create provisions a client. Only a platform admin (an admin in Reportage,
// the first workspace) may, since every new client is a commercial decision.
// The creator becomes the new workspace's first admin so somebody can invite
// the rest; a system actor leaves it to be populated through AddMember.
func (w *Workspaces) Create(ctx context.Context, actor core.Actor, in WorkspaceInput) (Workspace, error) {
	if !w.platformAdmin(actor) {
		return Workspace{}, core.ErrForbidden
	}
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	name := strings.TrimSpace(in.Name)
	switch {
	case slug == "" || len(slug) > 40 || !slugPattern.MatchString(slug):
		return Workspace{}, core.Invalid("slug must be 1-40 lower-case letters, digits and single hyphens")
	case name == "" || utf8.RuneCountInString(name) > 100:
		return Workspace{}, core.Invalid("name must be 1-100 characters")
	}
	industry := strings.TrimSpace(in.Industry)
	if industry != "" {
		if err := validateDocument(IndustryDocument{Industry: industry}); err != nil {
			return Workspace{}, err
		}
	}
	now := w.now().UTC()
	var row store.Workspace
	err := w.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		row, err = q.CreateWorkspace(ctx, store.CreateWorkspaceParams{Slug: slug, Name: name, CreatedAt: now})
		if err != nil {
			return err
		}
		if actor.UserID != 0 {
			if err := q.CreateMembership(ctx, store.CreateMembershipParams{
				WorkspaceID: row.ID, UserID: actor.UserID, Role: string(core.RoleAdmin), Enabled: 1, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if industry != "" {
			doc, _ := json.Marshal(IndustryDocument{Industry: industry, Vocabulary: map[string]string{}, Stages: []string{}, Qualifiers: []string{}})
			if _, err := q.CreateIndustryConfig(ctx, store.CreateIndustryConfigParams{
				WorkspaceID: row.ID, Version: 1, Document: string(doc), CreatedBy: optionalID(actor.UserID), CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if store.IsUniqueViolation(err) {
		return Workspace{}, core.Conflict("a workspace with slug %q already exists", slug)
	}
	if err != nil {
		return Workspace{}, err
	}
	role := core.RoleAdmin
	return Workspace{ID: row.ID, Slug: row.Slug, Name: row.Name, Role: role, Current: false, CreatedAt: row.CreatedAt}, nil
}

// Mine lists the workspaces the actor may switch to: their enabled
// memberships, or every workspace for a system actor and the dev admin,
// who have no person behind them.
func (w *Workspaces) Mine(ctx context.Context, actor core.Actor) ([]Workspace, error) {
	if !actor.AtLeast(core.RoleViewer) {
		return nil, core.ErrForbidden
	}
	out := []Workspace{}
	if actor.UserID == 0 {
		rows, err := w.db.Reader().ListWorkspaces(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, Workspace{ID: r.ID, Slug: r.Slug, Name: r.Name, Role: actor.Role, Current: r.ID == actor.WorkspaceID, CreatedAt: r.CreatedAt})
		}
		return out, nil
	}
	rows, err := w.db.Reader().ListMembershipsForUser(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, Workspace{ID: r.WorkspaceID, Slug: r.WorkspaceSlug, Name: r.WorkspaceName, Role: core.Role(r.Role), Current: r.WorkspaceID == actor.WorkspaceID, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// Members lists who may act in the actor's workspace. Admins only: the list
// is the client's staff, not something a viewer browses.
func (w *Workspaces) Members(ctx context.Context, actor core.Actor) ([]Member, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := w.db.Reader().ListMembershipsForWorkspace(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(rows))
	for _, r := range rows {
		var name *string
		if r.UserName.Valid {
			n := r.UserName.String
			name = &n
		}
		out = append(out, Member{UserID: r.UserID, Email: r.Email, Name: name, Role: core.Role(r.Role), Enabled: r.Enabled == 1})
	}
	return out, nil
}

// AddMember gives an existing ADX person a role in the actor's workspace, or
// changes the role they have. The person must already exist (users.Add
// creates people); an admin may grant at most their own role, so nobody
// hands out more than they hold. The assistant never does this.
func (w *Workspaces) AddMember(ctx context.Context, actor core.Actor, in MemberInput) (Member, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Member{}, core.ErrForbidden
	}
	if !in.Role.Valid() {
		return Member{}, core.Invalid("role must be viewer, marketer or admin")
	}
	if !actor.AtLeast(in.Role) {
		return Member{}, &core.Error{Kind: core.KindForbidden, Message: "you can't grant a role above your own"}
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	user, err := w.db.Reader().GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return Member{}, core.NotFound("nobody in ADX has the email %q; add them under People first", email)
	}
	if err != nil {
		return Member{}, err
	}
	now := w.now().UTC()
	err = w.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		_, err := q.GetMembership(ctx, store.GetMembershipParams{WorkspaceID: actor.WorkspaceID, UserID: user.ID})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return q.CreateMembership(ctx, store.CreateMembershipParams{
				WorkspaceID: actor.WorkspaceID, UserID: user.ID, Role: string(in.Role), Enabled: 1, CreatedAt: now,
			})
		case err != nil:
			return err
		}
		if err := q.SetMembershipRole(ctx, store.SetMembershipRoleParams{Role: string(in.Role), WorkspaceID: actor.WorkspaceID, UserID: user.ID}); err != nil {
			return err
		}
		return q.SetMembershipEnabled(ctx, store.SetMembershipEnabledParams{Enabled: 1, WorkspaceID: actor.WorkspaceID, UserID: user.ID})
	})
	if err != nil {
		return Member{}, err
	}
	var name *string
	if user.Name.Valid {
		n := user.Name.String
		name = &n
	}
	return Member{UserID: user.ID, Email: user.Email, Name: name, Role: in.Role, Enabled: true}, nil
}

// RemoveMember disables a membership in the actor's workspace. The person
// keeps their ADX account and their other workspaces. The last enabled admin
// of a workspace stays, or nobody could administer it.
func (w *Workspaces) RemoveMember(ctx context.Context, actor core.Actor, userID int64) error {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return core.ErrForbidden
	}
	return w.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		member, err := q.GetMembership(ctx, store.GetMembershipParams{WorkspaceID: actor.WorkspaceID, UserID: userID})
		if errors.Is(err, sql.ErrNoRows) {
			return core.NotFound("user %d is not a member of this workspace", userID)
		}
		if err != nil {
			return err
		}
		if member.Role == string(core.RoleAdmin) && member.Enabled == 1 {
			rows, err := q.ListMembershipsForWorkspace(ctx, actor.WorkspaceID)
			if err != nil {
				return err
			}
			admins := 0
			for _, r := range rows {
				if r.Role == string(core.RoleAdmin) && r.Enabled == 1 {
					admins++
				}
			}
			if admins <= 1 {
				return core.Conflict("this workspace needs at least one enabled admin")
			}
		}
		return q.SetMembershipEnabled(ctx, store.SetMembershipEnabledParams{Enabled: 0, WorkspaceID: actor.WorkspaceID, UserID: userID})
	})
}

// Industry returns the current industry configuration of the actor's
// workspace and the versions that exist. No configuration is "no
// configuration", never a default pretending to be one.
func (w *Workspaces) Industry(ctx context.Context, actor core.Actor) (IndustryState, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return IndustryState{}, core.ErrForbidden
	}
	rows, err := w.db.Reader().ListIndustryConfigs(ctx, actor.WorkspaceID)
	if err != nil {
		return IndustryState{}, err
	}
	state := IndustryState{Versions: []int64{}}
	for i, r := range rows {
		state.Versions = append(state.Versions, r.Version)
		if i == 0 {
			cfg, err := fromRow(r)
			if err != nil {
				return IndustryState{}, err
			}
			state.Current = &cfg
		}
	}
	return state, nil
}

// IndustryVersion returns one version exactly as it was written, for history
// that must be read with the definitions it was made under.
func (w *Workspaces) IndustryVersion(ctx context.Context, actor core.Actor, version int64) (IndustryConfig, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return IndustryConfig{}, core.ErrForbidden
	}
	row, err := w.db.Reader().GetIndustryConfigVersion(ctx, store.GetIndustryConfigVersionParams{WorkspaceID: actor.WorkspaceID, Version: version})
	if errors.Is(err, sql.ErrNoRows) {
		return IndustryConfig{}, core.NotFound("industry configuration version %d does not exist", version)
	}
	if err != nil {
		return IndustryConfig{}, err
	}
	return fromRow(row)
}

// PutIndustry writes a new version of the workspace's configuration. Versions
// are never edited in place: whatever was assessed under version 1 still says
// version 1 and still reads as version 1 did. Admins only.
func (w *Workspaces) PutIndustry(ctx context.Context, actor core.Actor, doc IndustryDocument) (IndustryConfig, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return IndustryConfig{}, core.ErrForbidden
	}
	doc = normalize(doc)
	if err := validateDocument(doc); err != nil {
		return IndustryConfig{}, err
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return IndustryConfig{}, err
	}
	now := w.now().UTC()
	var row store.IndustryConfig
	err = w.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		version := int64(1)
		current, err := q.GetCurrentIndustryConfig(ctx, actor.WorkspaceID)
		switch {
		case err == nil:
			version = current.Version + 1
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		row, err = q.CreateIndustryConfig(ctx, store.CreateIndustryConfigParams{
			WorkspaceID: actor.WorkspaceID, Version: version, Document: string(raw), CreatedBy: optionalID(actor.UserID), CreatedAt: now,
		})
		return err
	})
	if err != nil {
		return IndustryConfig{}, err
	}
	return fromRow(row)
}

// RejectPermissionKeys refuses documents that smuggle permission-shaped keys
// in through the raw JSON, which the typed struct would silently drop.
func RejectPermissionKeys(raw json.RawMessage) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return core.Invalid("industry configuration must be a JSON object")
	}
	for k := range m {
		switch strings.ToLower(k) {
		case "role", "roles", "permission", "permissions", "workspaceid", "workspace_id", "members", "admins":
			return core.Invalid("industry configuration cannot set %q: it configures vocabulary and stages, never who may do what", k)
		}
	}
	return nil
}

func normalize(doc IndustryDocument) IndustryDocument {
	doc.Industry = strings.TrimSpace(doc.Industry)
	doc.Notes = strings.TrimSpace(doc.Notes)
	if doc.Vocabulary == nil {
		doc.Vocabulary = map[string]string{}
	}
	if doc.Stages == nil {
		doc.Stages = []string{}
	}
	if doc.Qualifiers == nil {
		doc.Qualifiers = []string{}
	}
	for i, s := range doc.Stages {
		doc.Stages[i] = strings.TrimSpace(s)
	}
	for i, s := range doc.Qualifiers {
		doc.Qualifiers[i] = strings.TrimSpace(s)
	}
	return doc
}

func validateDocument(doc IndustryDocument) error {
	if doc.Industry == "" || utf8.RuneCountInString(doc.Industry) > 60 {
		return core.Invalid("industry must be 1-60 characters")
	}
	if utf8.RuneCountInString(doc.Notes) > 2000 {
		return core.Invalid("notes must be 2000 characters or fewer")
	}
	seen := map[string]bool{}
	for _, s := range doc.Stages {
		if s == "" || utf8.RuneCountInString(s) > 60 {
			return core.Invalid("every stage needs a name of 60 characters or fewer")
		}
		if seen[strings.ToLower(s)] {
			return core.Invalid("stage %q is listed twice", s)
		}
		seen[strings.ToLower(s)] = true
	}
	for _, s := range doc.Qualifiers {
		if s == "" || utf8.RuneCountInString(s) > 120 {
			return core.Invalid("every qualifier needs a text of 120 characters or fewer")
		}
	}
	for k, v := range doc.Vocabulary {
		if strings.TrimSpace(k) == "" || utf8.RuneCountInString(k) > 60 || utf8.RuneCountInString(v) > 200 {
			return core.Invalid("vocabulary entries need a term of 60 characters or fewer and a meaning of 200 or fewer")
		}
	}
	return nil
}

func fromRow(r store.IndustryConfig) (IndustryConfig, error) {
	var doc IndustryDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return IndustryConfig{}, err
	}
	doc = normalize(doc)
	var by *int64
	if r.CreatedBy.Valid {
		id := r.CreatedBy.Int64
		by = &id
	}
	return IndustryConfig{Version: r.Version, Document: doc, CreatedBy: by, CreatedAt: r.CreatedAt}, nil
}

func (w *Workspaces) platformAdmin(actor core.Actor) bool {
	return actor.AtLeast(core.RoleAdmin) && actor.WorkspaceID == core.DefaultWorkspace && actor.Via != core.ViaAssistant
}

func optionalID(id int64) sql.NullInt64 {
	return sql.NullInt64{Int64: id, Valid: id != 0}
}
