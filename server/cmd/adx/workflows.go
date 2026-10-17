package main

import (
	"context"
	"os"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/workflows"
)

// workflowReads maps the read capabilities a workflow step may call to the
// same core functions Ask ADX uses. Every one of them reads.
func workflowReads(rep *reporting.Reporting, l *leads.Leads, al *alerts.Alerts, conn *connections.Connections) map[string]workflows.ReadTool {
	reads := map[string]workflows.ReadTool{}
	if rep != nil {
		reads["get_analytics"] = func(ctx context.Context, actor core.Actor, in map[string]any) (any, error) {
			return rep.Analytics(ctx, actor, reporting.AnalyticsQuery{From: reporting.Day(str(in["from"])), To: reporting.Day(str(in["to"])), GroupBy: str(in["group"]), Compare: str(in["compare"])})
		}
	}
	if l != nil {
		reads["get_lead_coverage"] = func(ctx context.Context, actor core.Actor, _ map[string]any) (any, error) {
			return l.Coverage(ctx, actor, 0)
		}
	}
	if al != nil {
		reads["list_alerts"] = func(ctx context.Context, actor core.Actor, in map[string]any) (any, error) {
			return al.List(ctx, actor, str(in["state"]), 50)
		}
	}
	if conn != nil {
		reads["get_connections"] = func(ctx context.Context, actor core.Actor, _ map[string]any) (any, error) {
			return conn.ListAccounts(ctx, actor)
		}
	}
	return reads
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "adx"
	}
	return h
}
