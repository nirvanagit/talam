package enrich

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

type fakeToolCaller struct {
	calls   []string
	result  string
	err     error
	lastArg map[string]any
}

func (f *fakeToolCaller) CallTool(ctx context.Context, toolset, tool string, args map[string]any) (string, error) {
	f.calls = append(f.calls, toolset+"/"+tool)
	f.lastArg = args
	if f.err != nil {
		return "", f.err
	}
	return f.result, nil
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestEnrichAddsEvidenceForKnownAnalyzer(t *testing.T) {
	tools := &fakeToolCaller{result: `{"host":"httpbin"}`}
	e := &Enricher{Tools: tools, Log: testLogger()}
	inc := api.Incident{Findings: []mesh.Finding{{
		AnalyzerID:  "istio.destinationrule.orphaned-subset",
		Resource:    mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"},
		RawEvidence: map[string]any{"subset": "v2"},
	}}}

	out := e.Enrich(context.Background(), inc)

	if len(tools.calls) != 1 || tools.calls[0] != "mesh/get_destination_rule" {
		t.Fatalf("expected exactly one call to mesh/get_destination_rule, got %v", tools.calls)
	}
	if tools.lastArg["namespace"] != "demo" || tools.lastArg["name"] != "httpbin" {
		t.Errorf("expected args built from the finding's own resource, got %+v", tools.lastArg)
	}
	enrichment, ok := out.Findings[0].RawEvidence["mcpEnrichment"]
	if !ok {
		t.Fatal("expected mcpEnrichment key added to evidence")
	}
	results := enrichment.([]map[string]any)
	if results[0]["tool"] != "get_destination_rule" || results[0]["result"] != `{"host":"httpbin"}` {
		t.Errorf("unexpected enrichment content: %+v", results)
	}
	// The original evidence key must survive alongside the new one.
	if out.Findings[0].RawEvidence["subset"] != "v2" {
		t.Error("enrichment must not drop existing evidence keys")
	}
}

func TestEnrichLeavesUnknownAnalyzerUntouched(t *testing.T) {
	tools := &fakeToolCaller{result: "should never be called"}
	e := &Enricher{Tools: tools, Log: testLogger()}
	inc := api.Incident{Findings: []mesh.Finding{{AnalyzerID: "some.unmapped.analyzer", RawEvidence: map[string]any{"x": 1}}}}

	out := e.Enrich(context.Background(), inc)

	if len(tools.calls) != 0 {
		t.Fatalf("expected no MCP calls for an analyzer with no table entry, got %v", tools.calls)
	}
	if len(out.Findings[0].RawEvidence) != 1 {
		t.Errorf("evidence should be unmodified, got %+v", out.Findings[0].RawEvidence)
	}
}

func TestEnrichToolFailureIsNonFatal(t *testing.T) {
	tools := &fakeToolCaller{err: fmt.Errorf("connection refused")}
	e := &Enricher{Tools: tools, Log: testLogger()}
	inc := api.Incident{Findings: []mesh.Finding{{
		AnalyzerID: "istio.destinationrule.orphaned-subset",
		Resource:   mesh.ResourceRef{Namespace: "demo", Name: "httpbin"},
	}}}

	out := e.Enrich(context.Background(), inc)

	if _, ok := out.Findings[0].RawEvidence["mcpEnrichment"]; ok {
		t.Error("a failed tool call must not add an mcpEnrichment key at all")
	}
}

func TestEnrichNilEnricherIsNoOp(t *testing.T) {
	var e *Enricher
	inc := api.Incident{Findings: []mesh.Finding{{AnalyzerID: "istio.destinationrule.orphaned-subset"}}}
	out := e.Enrich(context.Background(), inc)
	if len(out.Findings) != 1 {
		t.Fatal("nil Enricher must safely pass the incident through unchanged")
	}
}

func TestEnrichNilToolsIsNoOp(t *testing.T) {
	e := &Enricher{Tools: nil, Log: testLogger()}
	inc := api.Incident{Findings: []mesh.Finding{{AnalyzerID: "istio.destinationrule.orphaned-subset"}}}
	out := e.Enrich(context.Background(), inc)
	if _, ok := out.Findings[0].RawEvidence["mcpEnrichment"]; ok {
		t.Error("an Enricher with no ToolCaller configured must not attempt any call")
	}
}
