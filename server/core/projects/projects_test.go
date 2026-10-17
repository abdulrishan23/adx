package projects_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/store/storetest"
)

var (
	admin  = core.Actor{UserID: 1, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	viewer = core.Actor{UserID: 2, Role: core.RoleViewer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
)

func TestCreateAndList(t *testing.T) {
	p := projects.New(storetest.Open(t))

	_, err := p.Create(t.Context(), admin, projects.CreateInput{Name: "  The Wadi "})
	require.NoError(t, err)
	_, err = p.Create(t.Context(), admin, projects.CreateInput{Name: "Oceana"})
	require.NoError(t, err)

	list, err := p.List(t.Context(), viewer)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "Oceana", list[0].Name)
	assert.Equal(t, "The Wadi", list[1].Name)
}

func TestCreateRejects(t *testing.T) {
	p := projects.New(storetest.Open(t))
	_, err := p.Create(t.Context(), admin, projects.CreateInput{Name: "The Wadi"})
	require.NoError(t, err)

	tests := []struct {
		name  string
		actor core.Actor
		input string
		kind  core.Kind
	}{
		{"viewer can't create", viewer, "Verdana", core.KindForbidden},
		{"empty name", admin, "   ", core.KindInvalid},
		{"same name in another case", admin, "the wadi", core.KindConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Create(t.Context(), tt.actor, projects.CreateInput{Name: tt.input})
			var ce *core.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, tt.kind, ce.Kind)
		})
	}
}

func TestListNeedsARole(t *testing.T) {
	p := projects.New(storetest.Open(t))
	_, err := p.List(t.Context(), core.Actor{})
	assert.ErrorIs(t, err, core.ErrForbidden)
}
