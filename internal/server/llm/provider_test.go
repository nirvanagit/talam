package llm

import (
	"context"
	"testing"

	"github.com/nirvanagit/talam/pkg/api"
	"github.com/nirvanagit/talam/pkg/mesh"
)

// fakeProvider returns a canned response, letting gateway tests run without
// network access — the schema validation is what's under test, not any
// particular model's behavior.
type fakeProvider struct {
	response string
	err      error
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Complete(ctx context.Context, system, user string) (string, error) {
	return f.response, f.err
}

func testIncident() api.Incident {
	return api.Incident{
		ID: "inc-1",
		Findings: []mesh.Finding{{
			AnalyzerID: "istio.destinationrule.orphaned-subset",
			Severity:   mesh.SeverityCritical,
			Resource:   mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"},
			Cluster:    "kind",
		}},
	}
}

func TestProposeRejectsMalformedJSON(t *testing.T) {
	g := &Gateway{ProviderFunc: func() Provider { return &fakeProvider{response: "not json at all"} }}
	_, err := g.Propose(context.Background(), testIncident(), "explanation")
	if err == nil {
		t.Fatal("expected malformed JSON to be rejected at the schema boundary")
	}
}

func TestProposeRejectsWriteOutsideSpec(t *testing.T) {
	g := &Gateway{ProviderFunc: func() Provider {
		return &fakeProvider{response: `{"summary":"x","riskTier":"Low","target":{"kind":"DestinationRule","namespace":"demo","name":"httpbin"},"patch":[{"op":"replace","path":"/metadata/name","value":"evil"}]}`}
	}}
	_, err := g.Propose(context.Background(), testIncident(), "explanation")
	if err == nil {
		t.Fatal("a patch touching /metadata must be rejected — patches must stay under /spec")
	}
}

func TestProposeRejectsUnpatchableKind(t *testing.T) {
	g := &Gateway{ProviderFunc: func() Provider {
		return &fakeProvider{response: `{"summary":"x","riskTier":"Low","target":{"kind":"Secret","namespace":"demo","name":"creds"},"patch":[{"op":"replace","path":"/spec/x","value":1}]}`}
	}}
	_, err := g.Propose(context.Background(), testIncident(), "explanation")
	if err == nil {
		t.Fatal("Secret must never be a valid remediation target")
	}
}

func TestProposeAcceptsValidPatch(t *testing.T) {
	g := &Gateway{ProviderFunc: func() Provider {
		return &fakeProvider{response: "```json\n" + `{"summary":"remove orphaned subset","riskTier":"Low","target":{"kind":"DestinationRule","namespace":"demo","name":"httpbin"},"patch":[{"op":"remove","path":"/spec/subsets/1"}]}` + "\n```"}
	}}
	p, err := g.Propose(context.Background(), testIncident(), "explanation")
	if err != nil {
		t.Fatalf("valid proposal wrapped in a code fence should still parse: %v", err)
	}
	if p.RiskTier != api.RiskLow || p.Target.Name != "httpbin" {
		t.Errorf("unexpected proposal: %+v", p)
	}
}

func TestProposeHonorsNoPatch(t *testing.T) {
	g := &Gateway{ProviderFunc: func() Provider {
		return &fakeProvider{response: `{"noPatch": true, "reason": "requires manual investigation"}`}
	}}
	_, err := g.Propose(context.Background(), testIncident(), "explanation")
	if _, ok := err.(ErrNoPatch); !ok {
		t.Fatalf("expected ErrNoPatch, got %v (%T)", err, err)
	}
}

func TestExplainRejectsEmptyResponse(t *testing.T) {
	g := &Gateway{ProviderFunc: func() Provider { return &fakeProvider{response: "   "} }}
	_, err := g.Explain(context.Background(), testIncident())
	if err == nil {
		t.Fatal("an empty explanation should be treated as a failed call")
	}
}
