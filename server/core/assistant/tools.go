package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/reporting"
)

// Services are the core functions the read tools call. Every call carries
// the conversation's actor, so a viewer's assistant sees what a viewer sees.
type Services struct {
	Reporting   *reporting.Reporting
	Leads       *leads.Leads
	Alerts      *alerts.Alerts
	Connections *connections.Connections
}

// ToolResult is what a tool returns: a JSON document for the model, the
// evidence references that back it, and a safe error when it failed.
type ToolResult struct {
	Output       any           `json:"output"`
	EvidenceRefs []EvidenceRef `json:"evidenceRefs"`
	Error        string        `json:"error,omitempty"`
}

// EvidenceRef points at where a figure came from, so the screen can offer
// drill-down and the answer can be checked.
type EvidenceRef struct {
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Detail string `json:"detail,omitempty"`
}

type tool struct {
	spec ToolSpec
	// run is the deterministic implementation. input is already validated
	// against the schema's required fields and types.
	run func(ctx context.Context, actor core.Actor, s Services, input map[string]any) (ToolResult, error)
}

// registry is the allowlist. Nothing outside it can be called, whatever a
// prompt, a model or a tool result says. Every tool is a read.
func registry() []tool {
	return []tool{
		{
			spec: ToolSpec{Name: "get_analytics", Description: "Deterministic figures for a period: spend, leads, qualified leads, deals, cost per lead, Google conversions and Meta leads kept apart, with coverage and freshness. Days are Dubai days. Use this for any number.",
				InputSchema: schema(map[string]any{
					"from":    prop("string", "Start day, YYYY-MM-DD"),
					"to":      prop("string", "End day, YYYY-MM-DD"),
					"group":   enumProp("Grouping", "campaign", "project", "platform", "day"),
					"compare": enumProp("Comparison period", "", "previous", "year"),
				}, "from", "to")},
			run: func(ctx context.Context, actor core.Actor, s Services, in map[string]any) (ToolResult, error) {
				if s.Reporting == nil {
					return unavailable("analytics are not available on this server"), nil
				}
				res, err := s.Reporting.Analytics(ctx, actor, reporting.AnalyticsQuery{From: reporting.Day(str(in, "from")), To: reporting.Day(str(in, "to")),
					GroupBy: str(in, "group"), Compare: str(in, "compare")})
				if err != nil {
					return failed(err), nil
				}
				refs := []EvidenceRef{{Kind: "formula", Ref: res.FormulaVersion, Detail: "reporting formula version"}}
				if res.Freshness.ObservedThrough != nil {
					refs = append(refs, EvidenceRef{Kind: "freshness", Ref: string(*res.Freshness.ObservedThrough), Detail: "figures observed through"})
				}
				for _, r := range res.Rows {
					if r.CampaignID != nil {
						refs = append(refs, EvidenceRef{Kind: "campaign", Ref: fmt.Sprintf("%d", *r.CampaignID), Detail: r.Label})
					}
				}
				return ToolResult{Output: res, EvidenceRefs: refs}, nil
			},
		},
		{
			spec: ToolSpec{Name: "get_lead_coverage", Description: "Lead data quality: how many leads have a status, a stage, a project, a phone; how many share a phone; attribution match levels; whether the funnel is configured.",
				InputSchema: schema(map[string]any{}),
			},
			run: func(ctx context.Context, actor core.Actor, s Services, _ map[string]any) (ToolResult, error) {
				if s.Leads == nil {
					return unavailable("leads are not available on this server"), nil
				}
				cov, err := s.Leads.Coverage(ctx, actor, 0)
				if err != nil {
					return failed(err), nil
				}
				return ToolResult{Output: cov, EvidenceRefs: []EvidenceRef{{Kind: "leads", Ref: "coverage", Detail: "GoHighLevel leads as last imported"}}}, nil
			},
		},
		{
			spec: ToolSpec{Name: "list_alerts", Description: "Open, acknowledged or resolved alerts from the configured anomaly checks, with what was observed. Alerts never change anything.",
				InputSchema: schema(map[string]any{"state": enumProp("Alert state", "", "open", "acknowledged", "resolved")}),
			},
			run: func(ctx context.Context, actor core.Actor, s Services, in map[string]any) (ToolResult, error) {
				if s.Alerts == nil {
					return unavailable("alerts are not available on this server"), nil
				}
				list, err := s.Alerts.List(ctx, actor, str(in, "state"), 50)
				if err != nil {
					return failed(err), nil
				}
				refs := make([]EvidenceRef, 0, len(list))
				for _, a := range list {
					refs = append(refs, EvidenceRef{Kind: "alert", Ref: fmt.Sprintf("%d", a.ID), Detail: a.Check + " " + a.SubjectLabel})
				}
				return ToolResult{Output: list, EvidenceRefs: refs}, nil
			},
		},
		{
			spec: ToolSpec{Name: "get_connections", Description: "Connected accounts with their health and, for ad accounts, how fresh their figures are.",
				InputSchema: schema(map[string]any{}),
			},
			run: func(ctx context.Context, actor core.Actor, s Services, _ map[string]any) (ToolResult, error) {
				out := map[string]any{}
				var refs []EvidenceRef
				if s.Connections != nil {
					accounts, err := s.Connections.ListAccounts(ctx, actor)
					if err != nil {
						return failed(err), nil
					}
					out["accounts"] = accounts
					for _, a := range accounts {
						refs = append(refs, EvidenceRef{Kind: "account", Ref: fmt.Sprintf("%d", a.ID), Detail: a.Provider + " " + a.ExternalID})
					}
				} else {
					out["accounts"] = []any{}
				}
				if s.Reporting != nil {
					fresh, err := s.Reporting.ListFreshness(ctx, actor)
					if err != nil {
						return failed(err), nil
					}
					out["freshness"] = fresh
				}
				return ToolResult{Output: out, EvidenceRefs: refs}, nil
			},
		},
	}
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func prop(typ, description string) map[string]any {
	return map[string]any{"type": typ, "description": description}
}

func enumProp(description string, values ...string) map[string]any {
	vs := make([]any, 0, len(values))
	for _, v := range values {
		vs = append(vs, v)
	}
	return map[string]any{"type": "string", "description": description, "enum": vs}
}

func str(in map[string]any, key string) string {
	v, _ := in[key].(string)
	return v
}

func failed(err error) ToolResult {
	var ce *core.Error
	if errorsAs(err, &ce) {
		return ToolResult{Error: ce.Message}
	}
	return ToolResult{Error: "the tool failed; see the server log"}
}

func unavailable(msg string) ToolResult { return ToolResult{Error: msg} }

// validateInput checks a tool call against its schema: unknown fields,
// missing required fields, wrong types and values outside an enum are
// refused before anything runs.
func validateInput(spec ToolSpec, raw json.RawMessage) (map[string]any, error) {
	in := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, fmt.Errorf("input is not a JSON object")
		}
	}
	props, _ := spec.InputSchema["properties"].(map[string]any)
	required, _ := spec.InputSchema["required"].([]string)
	for _, r := range required {
		if _, ok := in[r]; !ok {
			return nil, fmt.Errorf("missing required field %q", r)
		}
	}
	for k, v := range in {
		p, ok := props[k].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unknown field %q", k)
		}
		switch p["type"] {
		case "string":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("field %q must be a string", k)
			}
			if enum, ok := p["enum"].([]any); ok {
				found := false
				for _, e := range enum {
					if e == s {
						found = true
					}
				}
				if !found {
					return nil, fmt.Errorf("field %q must be one of %s", k, joinAny(enum))
				}
			}
		case "integer", "number":
			if _, ok := v.(float64); !ok {
				return nil, fmt.Errorf("field %q must be a number", k)
			}
		case "boolean":
			if _, ok := v.(bool); !ok {
				return nil, fmt.Errorf("field %q must be true or false", k)
			}
		}
	}
	return in, nil
}

func joinAny(vs []any) string {
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		parts = append(parts, fmt.Sprintf("%q", v))
	}
	return strings.Join(parts, ", ")
}
