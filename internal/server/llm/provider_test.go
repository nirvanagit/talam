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
			AnalyzerID:  "istio.destinationrule.orphaned-subset",
			Severity:    mesh.SeverityCritical,
			Resource:    mesh.ResourceRef{Kind: "DestinationRule", Namespace: "demo", Name: "httpbin"},
			Cluster:     "kind",
			RawEvidence: map[string]any{"resourceVersion": "123"},
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
	// The resourceVersion must come from talam's own collected evidence, not
	// anything the model could influence — the model never even sees a field
	// named that way to echo back.
	if p.TargetResourceVersion != "123" {
		t.Errorf("expected TargetResourceVersion pinned from evidence, got %q", p.TargetResourceVersion)
	}
}

func TestProposeRejectsPathThatOnlyLooksLikeSpec(t *testing.T) {
	g := &Gateway{ProviderFunc: func() Provider {
		return &fakeProvider{response: `{"summary":"x","riskTier":"Low","target":{"kind":"DestinationRule","namespace":"demo","name":"httpbin"},"patch":[{"op":"replace","path":"/specialFieldThatIsntSpec","value":1}]}`}
	}}
	_, err := g.Propose(context.Background(), testIncident(), "explanation")
	if err == nil {
		t.Fatal("a path merely prefixed with the string \"/spec\" (not \"/spec\" or \"/spec/...\") must be rejected")
	}
}

func TestProposeRejectsTargetNotInEvidence(t *testing.T) {
	g := &Gateway{ProviderFunc: func() Provider {
		// Targets a resource name the incident's evidence never mentioned.
		return &fakeProvider{response: `{"summary":"x","riskTier":"Low","target":{"kind":"DestinationRule","namespace":"demo","name":"some-other-rule"},"patch":[{"op":"remove","path":"/spec/subsets/0"}]}`}
	}}
	_, err := g.Propose(context.Background(), testIncident(), "explanation")
	if err == nil {
		t.Fatal("a proposal targeting a resource absent from the incident's evidence must be rejected — it can't be pinned to a trusted resourceVersion")
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
