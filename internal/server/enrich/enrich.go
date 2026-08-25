// Package enrich is talam-server's deterministic MCP evidence-enrichment
// step (ADR-0006): a static table from analyzer ID to a fixed set of MCP
// tool calls, run before the LLM gateway ever sees a Finding. Nothing here
// lets the LLM choose a tool or its arguments — the mapping is as
// deterministic as an analyzer's own detection logic, it just runs
// server-side because MCP servers may not be reachable from every agent.
package enrich

import (
	"context"
	"log/slog"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

// ToolCaller is the one thing Enricher needs — satisfied by
// internal/server/mcp.Registry, and easy to fake in tests.
type ToolCaller interface {
	CallTool(ctx context.Context, toolset, tool string, args map[string]any) (string, error)
}

// call is one MCP tool invocation to make for a matching Finding, with its
// arguments built from that finding's own Resource/RelatedRefs — never from
// anything an LLM supplies.
type call struct {
	toolset string
	tool    string
	args    func(f mesh.Finding) map[string]any
}

// table maps AnalyzerID to the enrichment calls to make for it. Adding
// enrichment for a new analyzer is a new entry here, not a prompt change —
// see ADR-0006.
var table = map[string][]call{
	"istio.destinationrule.orphaned-subset": {
		{toolset: "mesh", tool: "get_destination_rule", args: func(f mesh.Finding) map[string]any {
			return map[string]any{"namespace": f.Resource.Namespace, "name": f.Resource.Name}
		}},
	},
	"istio.virtualservice.dangling-host": {
		{toolset: "mesh", tool: "list_service_entries", args: func(f mesh.Finding) map[string]any {
			return map[string]any{"namespace": f.Resource.Namespace}
		}},
	},
	"istio.gateway.unbound-selector": {
		{toolset: "mesh", tool: "get_mtls_status", args: func(f mesh.Finding) map[string]any {
			return map[string]any{"namespace": f.Resource.Namespace}
		}},
	},
}

type Enricher struct {
	Tools ToolCaller
	Log   *slog.Logger
}

// Enrich returns a copy of inc whose findings carry an extra
// RawEvidence["mcpEnrichment"] entry for every successful tool call the
// table has for that finding's analyzer. A tool call failing (server
// unreachable, tool missing) is logged and skipped — enrichment is always
// best-effort, never something that blocks an explanation from happening at
// all.
func (e *Enricher) Enrich(ctx context.Context, inc api.Incident) api.Incident {
	if e == nil || e.Tools == nil {
		return inc
	}
	out := inc
	out.Findings = make([]mesh.Finding, len(inc.Findings))
	for i, f := range inc.Findings {
		out.Findings[i] = e.enrichOne(ctx, f)
	}
	return out
}

func (e *Enricher) enrichOne(ctx context.Context, f mesh.Finding) mesh.Finding {
	calls := table[f.AnalyzerID]
	if len(calls) == 0 {
		return f
	}

	var results []map[string]any
	for _, c := range calls {
		text, err := e.Tools.CallTool(ctx, c.toolset, c.tool, c.args(f))
		if err != nil {
			e.Log.Warn("MCP enrichment call failed", "analyzer", f.AnalyzerID, "toolset", c.toolset, "tool", c.tool, "err", err)
			continue
		}
		e.Log.Info("MCP enrichment call succeeded", "analyzer", f.AnalyzerID, "toolset", c.toolset, "tool", c.tool, "resultBytes", len(text))
		results = append(results, map[string]any{"tool": c.tool, "result": text})
	}
	if len(results) == 0 {
		return f
	}

	evidence := make(map[string]any, len(f.RawEvidence)+1)
	for k, v := range f.RawEvidence {
		evidence[k] = v
	}
	evidence["mcpEnrichment"] = results
	f.RawEvidence = evidence
	return f
}
