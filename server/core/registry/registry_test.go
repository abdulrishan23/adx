package registry_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/registry"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type accounts []int64

func (a accounts) ConnectedAccountIDs(context.Context, core.Actor) ([]int64, error) { return a, nil }

func setup(t *testing.T) *registry.Registry {
	t.Helper()
	db := storetest.Open(t)
	coretest.SeedUsers(t, db)
	return registry.New(db, registry.Config{Clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)), Accounts: accounts{11, 12}})
}

// forbidden accepts both the bare permission error and a forbidden error
// that names what is missing; both are a 403 to the API.
func forbidden(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	if errors.Is(err, core.ErrForbidden) {
		return
	}
	var ce *core.Error
	require.True(t, errors.As(err, &ce), err.Error())
	assert.Equal(t, core.KindForbidden, ce.Kind)
}

func skill(tools ...string) registry.SkillDocument {
	return registry.SkillDocument{Name: "Research pass", Instructions: "Read the competitor page and list offers.", EvidenceScope: []string{"public"}, PermittedTools: tools,
		InputSchema:  map[string]any{"type": "object", "required": []string{"url"}, "properties": map[string]any{"url": map[string]any{"type": "string"}}},
		OutputSchema: map[string]any{"type": "object", "required": []string{"offers"}, "additionalProperties": false, "properties": map[string]any{"offers": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"name", "pct"}, "properties": map[string]any{"name": map[string]any{"type": "string"}, "pct": map[string]any{"type": "number", "minimum": 0, "maximum": 100}}}}, "again": map[string]any{"type": "boolean"}}}}
}

// Catches: a marketer activating a version that adds a tool or an account,
// a new version with the same scope needing an admin for no reason, and an
// agent naming an account that is not connected.
func TestPromptEditCannotExpandAccess(t *testing.T) {
	r := setup(t)
	ctx := t.Context()
	// A first version with tools is a grant: admin only.
	v1, err := r.SaveSkill(ctx, coretest.Marketer, "research.pass", skill("read_research"), false)
	require.NoError(t, err)
	assert.True(t, v1.NeedsAdmin)
	_, err = r.Activate(ctx, coretest.Marketer, v1.ID)
	forbidden(t, err)
	v1, err = r.Activate(ctx, coretest.Admin, v1.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", v1.State)
	assert.Contains(t, v1.Reason, "accepted by admin")

	// A prompt edit with the same tools and scope: the marketer activates it.
	edited := skill("read_research")
	edited.Instructions = "Read the competitor page, list offers and note launch dates."
	v2, err := r.SaveSkill(ctx, coretest.Marketer, "research.pass", edited, false)
	require.NoError(t, err)
	assert.False(t, v2.NeedsAdmin)
	assert.Equal(t, int64(2), v2.Version)
	v2, err = r.Activate(ctx, coretest.Marketer, v2.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", v2.State)
	list, _ := r.List(ctx, coretest.Viewer, "skill")
	require.Len(t, list, 2)
	assert.Equal(t, "retired", list[1].State, "one active version per key")

	// A prompt edit that also adds propose_change: needs an admin.
	wider := skill("read_research", "propose_change")
	wider.Instructions = "Read the page and propose the budget change it implies."
	v3, err := r.SaveSkill(ctx, coretest.Marketer, "research.pass", wider, false)
	require.NoError(t, err)
	assert.True(t, v3.NeedsAdmin)
	assert.Contains(t, v3.Reason, "adds tools propose_change")
	_, err = r.Activate(ctx, coretest.Marketer, v3.ID)
	forbidden(t, err)
	active, _ := r.Active(ctx, coretest.Viewer, "skill", "research.pass")
	assert.Equal(t, int64(2), active.Version, "v2 stays active")
	// Widening evidence scope needs an admin too.
	scoped := skill("read_research")
	scoped.EvidenceScope = []string{"public", "account:*"}
	v4, err := r.SaveSkill(ctx, coretest.Marketer, "research.pass", scoped, false)
	require.NoError(t, err)
	assert.Contains(t, v4.Reason, "widens evidence scope")

	// Agents: the account scope must be connected accounts; adding one needs an admin.
	a1, err := r.SaveAgent(ctx, coretest.Admin, "researcher", registry.AgentDocument{Name: "Researcher", Skills: []string{"research.pass"}, AccountScope: []int64{11}}, false)
	require.NoError(t, err)
	a1, err = r.Activate(ctx, coretest.Admin, a1.ID)
	require.NoError(t, err)
	tools, err := r.AgentTools(ctx, coretest.Viewer, "researcher")
	require.NoError(t, err)
	assert.Equal(t, []string{"read_research"}, tools, "an agent's tools are its skills' tools, nothing more")
	a2, err := r.SaveAgent(ctx, coretest.Marketer, "researcher", registry.AgentDocument{Name: "Researcher", Instructions: "Be thorough.", Skills: []string{"research.pass"}, AccountScope: []int64{11, 12}}, false)
	require.NoError(t, err)
	assert.True(t, a2.NeedsAdmin)
	assert.Contains(t, a2.Reason, "adds accounts [12]")
	_, err = r.Activate(ctx, coretest.Marketer, a2.ID)
	forbidden(t, err)
	a3, err := r.SaveAgent(ctx, coretest.Admin, "researcher", registry.AgentDocument{Name: "Researcher", Skills: []string{"research.pass"}, AccountScope: []int64{99}}, false)
	require.NoError(t, err)
	assert.Contains(t, a3.Validation[0], "account 99 is not connected")
	_, err = r.Activate(ctx, coretest.Admin, a3.ID)
	require.Error(t, err, "validation problems block activation")
	// A super agent coordinates and holds no skills.
	_, err = r.SaveAgent(ctx, coretest.Admin, "lead", registry.AgentDocument{Name: "Lead", Skills: []string{"research.pass"}, Coordinates: []string{"researcher"}}, false)
	require.NoError(t, err)
	bad, _ := r.List(ctx, coretest.Viewer, "agent")
	assert.Contains(t, bad[0].Validation[0], "super agent")
	_ = a1
}

// Catches: an output with a missing field, a wrong type, an out-of-range
// number or an unknown field being accepted; and a skill declaring a schema
// nobody can check.
func TestMalformedOutputFailsSchemaValidation(t *testing.T) {
	r := setup(t)
	ctx := t.Context()
	v, err := r.SaveSkill(ctx, coretest.Admin, "research.pass", skill("read_research"), false)
	require.NoError(t, err)
	_, err = r.Activate(ctx, coretest.Admin, v.ID)
	require.NoError(t, err)
	good := json.RawMessage(`{"offers":[{"name":"Spring","pct":10}],"again":false}`)
	problems, err := r.CheckOutput(ctx, coretest.Viewer, "research.pass", good)
	require.NoError(t, err)
	assert.Empty(t, problems)
	bad := json.RawMessage(`{"offers":[{"name":"Spring","pct":"ten"},{"pct":250}],"extra":1}`)
	problems, _ = r.CheckOutput(ctx, coretest.Viewer, "research.pass", bad)
	assert.ElementsMatch(t, []string{
		"$.offers[0].pct: must be a number",
		"$.offers[1]: missing required field \"name\"",
		"$.offers[1].pct: must be at most 100",
		"$: unknown field \"extra\"",
	}, problems)
	problems, _ = r.CheckOutput(ctx, coretest.Viewer, "research.pass", json.RawMessage(`not json`))
	assert.Equal(t, []string{"$: not valid JSON"}, problems)
	problems, _ = r.CheckOutput(ctx, coretest.Viewer, "research.pass", json.RawMessage(`[]`))
	assert.Equal(t, []string{"$: must be an object"}, problems)
	problems, _ = r.CheckInput(ctx, coretest.Viewer, "research.pass", json.RawMessage(`{}`))
	assert.Equal(t, []string{"$: missing required field \"url\""}, problems)
	_, err = r.CheckOutput(ctx, coretest.Viewer, "nope", good)
	require.Error(t, err)
	// A schema with an unsupported type or an unknown required field is a draft problem.
	odd := skill("read_research")
	odd.OutputSchema = map[string]any{"type": "object", "required": []string{"x"}, "properties": map[string]any{"y": map[string]any{"type": "date"}}}
	d, err := r.SaveSkill(ctx, coretest.Admin, "odd", odd, false)
	require.NoError(t, err)
	assert.Contains(t, d.Validation, "outputSchema.y: type must be object, array, string, integer, number, boolean or null")
	assert.Contains(t, d.Validation, "outputSchema: required field \"x\" is not a property")
	_, err = r.Activate(ctx, coretest.Admin, d.ID)
	require.Error(t, err)
	// Unknown tools and scopes are named.
	odd2 := skill("delete_everything")
	odd2.EvidenceScope = []string{"everything"}
	d2, _ := r.SaveSkill(ctx, coretest.Admin, "odd2", odd2, false)
	assert.Contains(t, d2.Validation, "tool \"delete_everything\" is not in the capability catalogue")
	assert.Contains(t, d2.Validation[len(d2.Validation)-1], "evidence scope \"everything\"")
}

// Catches: a model-proposed skill or workflow going live on a marketer's
// click, and a workflow with a cycle, an unbounded loop or a proposing step
// not marked external being activated.
func TestGeneratedSkillStaysDraftUntilAccepted(t *testing.T) {
	r := setup(t)
	ctx := t.Context()
	asModel := coretest.Marketer
	asModel.Via = core.ViaAssistant
	gen, err := r.SaveSkill(ctx, asModel, "offers.extract", skill("read_research"), false)
	require.NoError(t, err)
	assert.Equal(t, "model", gen.Source)
	assert.True(t, gen.NeedsAdmin)
	assert.Contains(t, gen.Reason, "proposed by a model")
	_, err = r.Activate(ctx, coretest.Marketer, gen.ID)
	forbidden(t, err)
	_, err = r.Activate(ctx, asModel, gen.ID)
	forbidden(t, err)
	active, _ := r.Active(ctx, coretest.Viewer, "skill", "offers.extract")
	assert.Nil(t, active)
	gen, err = r.Activate(ctx, coretest.Admin, gen.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", gen.State)
	// The same through the explicit flag, by a marketer.
	gen2, err := r.SaveSkill(ctx, coretest.Marketer, "offers.extract", skill("read_research"), true)
	require.NoError(t, err)
	assert.Equal(t, "model", gen2.Source)
	assert.True(t, gen2.NeedsAdmin)

	// Workflows: validation names what is wrong; a clean one activates.
	prop := skill("read_research", "propose_change")
	p, err := r.SaveSkill(ctx, coretest.Admin, "budget.suggest", prop, false)
	require.NoError(t, err)
	_, err = r.Activate(ctx, coretest.Admin, p.ID)
	require.NoError(t, err)
	wf := registry.WorkflowDocument{Name: "Weekly review", MaxSteps: 10, Steps: []registry.WorkflowStep{
		{Key: "a", Skill: "offers.extract", DependsOn: []string{"c"}},
		{Key: "b", Skill: "budget.suggest", DependsOn: []string{"a"}},
		{Key: "c", Skill: "offers.extract", DependsOn: []string{"b"}, Loop: &registry.LoopSpec{Max: 0}},
		{Key: "d", Agent: "ghost"},
	}}
	problems, err := r.ValidateWorkflow(ctx, coretest.Viewer, wf)
	require.NoError(t, err)
	joined := ""
	for _, p := range problems {
		joined += p + "\n"
	}
	assert.Contains(t, joined, "cycle: a -> c -> b -> a")
	assert.Contains(t, joined, "unbounded loops are refused")
	assert.Contains(t, joined, "must be marked external")
	assert.Contains(t, joined, "agent \"ghost\" has no active version")
	w, err := r.SaveWorkflow(ctx, coretest.Admin, "weekly", wf, false)
	require.NoError(t, err)
	_, err = r.Activate(ctx, coretest.Admin, w.ID)
	require.Error(t, err)
	good := registry.WorkflowDocument{Name: "Weekly review", MaxSteps: 10, Steps: []registry.WorkflowStep{
		{Key: "extract", Skill: "offers.extract", Loop: &registry.LoopSpec{Max: 3}},
		{Key: "suggest", Skill: "budget.suggest", DependsOn: []string{"extract"}, External: true},
	}}
	w2, err := r.SaveWorkflow(ctx, coretest.Marketer, "weekly", good, false)
	require.NoError(t, err)
	assert.Empty(t, w2.Validation)
	assert.True(t, w2.NeedsAdmin, "a step that proposes external changes needs an admin")
	_, err = r.Activate(ctx, coretest.Marketer, w2.ID)
	forbidden(t, err)
	w2, err = r.Activate(ctx, coretest.Admin, w2.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", w2.State)
	tooMany := good
	tooMany.MaxSteps = 2
	w3, _ := r.SaveWorkflow(ctx, coretest.Admin, "weekly", tooMany, false)
	assert.Contains(t, w3.Validation[0], "more than maxSteps 2")
	assert.Len(t, r.Catalog(), 12)
}
