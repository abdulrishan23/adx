// Package projects keeps the list of Reportage's projects, such as The Wadi,
// so the same project isn't spelled two ways across campaigns and leads.
package projects

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

type Project struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type CreateInput struct {
	Name string `json:"name"`
}

type Projects struct {
	db *store.DB
}

func New(db *store.DB) *Projects {
	return &Projects{db: db}
}

func (p *Projects) List(ctx context.Context, actor core.Actor) ([]Project, error) {
	if !actor.AtLeast(core.RoleViewer) {
		return nil, core.ErrForbidden
	}
	rows, err := p.db.Reader().ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

func (p *Projects) Create(ctx context.Context, actor core.Actor, in CreateInput) (Project, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return Project{}, core.ErrForbidden
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Project{}, core.Invalid("name is required")
	}
	if utf8.RuneCountInString(name) > 100 {
		return Project{}, core.Invalid("name must be 100 characters or fewer")
	}
	row, err := p.db.Writer().CreateProject(ctx, store.CreateProjectParams{
		Name:      name,
		CreatedAt: time.Now().UTC(),
	})
	if store.IsUniqueViolation(err) {
		return Project{}, core.Conflict("a project named %q already exists", name)
	}
	if err != nil {
		return Project{}, err
	}
	return fromRow(row), nil
}

func fromRow(r store.Project) Project {
	return Project{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt}
}
